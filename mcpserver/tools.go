package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"syscall"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ha1t/obsidian-web-bridge/vault"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	listDefaultLimit   = 30
	listMaxLimit       = 200
	searchDefaultLimit = 20
	searchMaxLimit     = 100

	msgDailyNotesOnly = DailyNotesDir + " には append_daily_memo で Memo 欄に追記してください"
	msgHashMismatch   = "別の場所で編集されています。read_note で読み直してから update_note してください"
	msgDailyCreated   = "DailyNote を新しく作成しました。他の端末への反映は次の同期（最長1時間）までかかります"
	msgUnsafeName     = "ファイル名に使えない文字が含まれています（\\ : * ? \" < > | や末尾のドット・空白、CON などの予約名は使えません）。別の名前にしてください"
	msgNameTooLong    = "ファイル名が長すぎます。タイトルを短くしてください"
	msgCreatedRemoved = "created 行が消えています。read_note の内容の frontmatter を保ったまま更新してください"
)

// windowsReserved は Windows で使えないデバイス名（拡張子を除いた部分を大文字小文字を区別せず比べる）。
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// checkSafeName は clean の各要素が Windows / macOS の端末でも同期できる名前かを判定する。
// Vault は複数の OS で同期されるため、Linux では作れても他の端末で壊れる名前を拒否する。
func checkSafeName(clean string) error {
	for _, elem := range strings.Split(clean, "/") {
		if strings.ContainsAny(elem, `\:*?"<>|`) || strings.ContainsFunc(elem, func(r rune) bool { return r < 0x20 }) {
			return &userError{msg: msgUnsafeName}
		}
		if strings.HasSuffix(elem, ".") || strings.HasSuffix(elem, " ") {
			return &userError{msg: msgUnsafeName}
		}
		stem, _, _ := strings.Cut(elem, ".")
		if windowsReserved[strings.ToUpper(strings.TrimRight(stem, " "))] {
			return &userError{msg: msgUnsafeName}
		}
	}
	return nil
}

// userError は Claude にそのまま見せる業務エラー。
type userError struct{ msg string }

func (e *userError) Error() string { return e.msg }

func userErrf(format string, args ...any) error {
	return &userError{msg: fmt.Sprintf(format, args...)}
}

// toolMessage は err を Claude に返す日本語メッセージにする。
// 想定外のエラーは内部の詳細（絶対パスなど）を出さず、ログにだけ残す。
func toolMessage(tool string, err error) string {
	var ue *userError
	if errors.As(err, &ue) {
		return ue.msg
	}
	log.Printf("mcp tool=%s internal error: %v", tool, err)
	return "サーバー内部でエラーが発生しました。時間をおいてもう一度試してください"
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// addTool は Out の出力スキーマを付けてツールを登録する。
// SDK に Out を直接渡すと、エラー時にもゼロ値が構造化出力として付くため、
// 出力スキーマだけ Out から作り、ハンドラーは any で返す。
// handler の返す note が空でなければ、テキスト出力の先頭に足す。
func addTool[In, Out any](s *mcp.Server, t *mcp.Tool, handler func(ctx context.Context, in In) (Out, string, error)) {
	schema, err := jsonschema.For[Out](nil)
	if err != nil {
		panic(fmt.Sprintf("output schema for %s: %v", t.Name, err))
	}
	t.OutputSchema = schema
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, note, err := handler(ctx, in)
		if err != nil {
			return errorResult(toolMessage(t.Name, err)), nil, nil
		}
		if note == "" {
			return nil, out, nil // SDK が JSON をテキスト出力にも入れる
		}
		b, err := json.Marshal(out)
		if err != nil {
			return errorResult(toolMessage(t.Name, err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: note},
			&mcp.TextContent{Text: string(b)},
		}}, out, nil
	})
}

func boolPtr(b bool) *bool { return &b }

var (
	readOnlyAnnotations    = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}
	writeAnnotations       = &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}
	destructiveAnnotations = &mcp.ToolAnnotations{DestructiveHint: boolPtr(true), OpenWorldHint: boolPtr(false)}
)

type tools struct{ opts Options }

func registerTools(s *mcp.Server, t *tools) {
	addTool(s, &mcp.Tool{
		Name:        "list_notes",
		Description: "フォルダ内のノートを更新日時の新しい順に一覧します。",
		Annotations: readOnlyAnnotations,
	}, t.listNotes)
	addTool(s, &mcp.Tool{
		Name:        "search_notes",
		Description: "ファイル名と本文を部分一致で検索します（大文字小文字は区別しません）。更新日時の新しい順に、マッチ箇所の前後の抜粋を返します。",
		Annotations: readOnlyAnnotations,
	}, t.searchNotes)
	addTool(s, &mcp.Tool{
		Name:        "read_note",
		Description: "ノートの全文（frontmatter を含む生テキスト）、パースした frontmatter、内容の hash、更新日時を返します。hash は update_note に渡します。",
		Annotations: readOnlyAnnotations,
	}, t.readNote)
	addTool(s, &mcp.Tool{
		Name:        "create_note",
		Description: "新しいノートを作ります。すでに存在する場合は失敗します。frontmatter の tags と source はサーバーが補います。",
		Annotations: writeAnnotations,
	}, t.createNote)
	addTool(s, &mcp.Tool{
		Name:        "append_note",
		Description: "既存のノートに追記します。heading を省略するとノート末尾に、指定するとその見出しのセクション末尾に追記します。frontmatter と既存の本文は変えません。content にトップレベルの見出し（# ）は入れず、見出しは ## 以下を使ってください（ノートのセクション構造が崩れるため）。",
		Annotations: writeAnnotations,
	}, t.appendNote)
	addTool(s, &mcp.Tool{
		Name:        "update_note",
		Description: "ノートの全文（frontmatter を含む）を置き換えます。read_note で得た hash が現在の内容と一致するときだけ書き込みます。",
		Annotations: destructiveAnnotations,
	}, t.updateNote)
	addTool(s, &mcp.Tool{
		Name:        "append_daily_memo",
		Description: DailyNotesDir + "/<日付>.md の # Memo セクションの末尾に追記します。ノートが無ければテンプレートから作ってから追記します。content にトップレベルの見出し（# ）は入れず、見出しは ## 以下を使ってください（DailyNote のセクション構造が崩れるため）。",
		Annotations: writeAnnotations,
	}, t.appendDailyMemo)
}

// ---- 入出力 ----

type listNotesIn struct {
	Folder    string `json:"folder,omitempty" jsonschema:"Vault ルートからのフォルダの相対パス。省略時はルート"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"サブフォルダも含めるか。既定 false"`
	Limit     int    `json:"limit,omitempty" jsonschema:"最大件数。既定 30、上限 200"`
}

type noteInfoOut struct {
	Path     string `json:"path"`
	Modified string `json:"modified" jsonschema:"更新日時（RFC3339）"`
	Size     int64  `json:"size" jsonschema:"バイト数"`
}

type listNotesOut struct {
	Notes []noteInfoOut `json:"notes"`
}

type searchNotesIn struct {
	Query  string `json:"query" jsonschema:"検索する文字列"`
	Folder string `json:"folder,omitempty" jsonschema:"検索するフォルダ（サブフォルダを含む）。省略時は Vault 全体"`
	Limit  int    `json:"limit,omitempty" jsonschema:"最大件数。既定 20、上限 100"`
}

type searchHitOut struct {
	Path     string `json:"path"`
	Modified string `json:"modified" jsonschema:"更新日時（RFC3339）"`
	Excerpt  string `json:"excerpt" jsonschema:"マッチ箇所の前後の抜粋。ファイル名だけが一致したときは空"`
}

type searchNotesOut struct {
	Hits []searchHitOut `json:"hits"`
}

type pathIn struct {
	Path string `json:"path" jsonschema:"Vault ルートからのノートの相対パス（例: Notes/タイトル.md）"`
}

type readNoteOut struct {
	Path        string          `json:"path"`
	Content     string          `json:"content" jsonschema:"frontmatter を含む生テキスト全文"`
	Frontmatter *map[string]any `json:"frontmatter" jsonschema:"パースした frontmatter。パースできなければ null"`
	Hash        string          `json:"hash" jsonschema:"content の SHA-256。update_note に渡す"`
	Modified    string          `json:"modified" jsonschema:"更新日時（RFC3339）"`
}

type createNoteIn struct {
	Path        string         `json:"path" jsonschema:"作成するノートの Vault ルートからの相対パス（例: Notes/タイトル.md）"`
	Frontmatter map[string]any `json:"frontmatter,omitempty" jsonschema:"frontmatter（任意）。tags と source はサーバーが補い、created と modified は取り除く"`
	Body        string         `json:"body" jsonschema:"本文（frontmatter を除く Markdown）"`
}

type pathOut struct {
	Path string `json:"path"`
}

type appendNoteIn struct {
	Path    string `json:"path" jsonschema:"追記するノートの Vault ルートからの相対パス"`
	Content string `json:"content" jsonschema:"追記する Markdown"`
	Heading string `json:"heading,omitempty" jsonschema:"追記先の見出し。# を含めて書く（例: # Memo、## 作業ログ）。省略時はノート末尾"`
}

type updateNoteIn struct {
	Path    string `json:"path" jsonschema:"置き換えるノートの Vault ルートからの相対パス"`
	Content string `json:"content" jsonschema:"frontmatter を含む新しい全文"`
	Hash    string `json:"hash" jsonschema:"read_note で得た hash"`
}

type updateNoteOut struct {
	Path string `json:"path"`
	Hash string `json:"hash" jsonschema:"書き込んだ内容の新しい hash"`
}

type appendDailyMemoIn struct {
	Content string `json:"content" jsonschema:"Memo セクションに追記する Markdown"`
	Date    string `json:"date,omitempty" jsonschema:"対象日（YYYY-MM-DD）。省略時は今日"`
}

type appendDailyMemoOut struct {
	Path    string `json:"path"`
	Created bool   `json:"created" jsonschema:"DailyNote を新しく作ったか"`
}

// ---- 共通処理 ----

func clampLimit(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}

func (t *tools) formatTime(tm time.Time) string {
	return tm.In(t.opts.location()).Format(time.RFC3339)
}

// cleanPath はノートのパスを正規化する。不正なら Claude 向けのエラーを返す。
func (t *tools) cleanPath(p string) (string, error) {
	clean, err := t.opts.Vault.CleanPath(p)
	if err != nil {
		return "", userErrf("パス `%s` は使えません。Vault ルートからの相対パスで、.md で終わり、.. や . で始まる要素を含まないパスを指定してください", p)
	}
	return clean, nil
}

// checkWritable は append_daily_memo 以外の書き込みツールで、clean に書いてよいかを判定する。
func (t *tools) checkWritable(clean string) error {
	switch t.opts.Policy.Classify(clean) {
	case FolderReadonly:
		first, _, _ := strings.Cut(clean, "/")
		return userErrf("`%s` は読み取り専用フォルダです", first)
	case FolderDailyNotes:
		return &userError{msg: msgDailyNotesOnly}
	}
	return nil
}

// noteErr は vault の既知のエラーを Claude 向けのメッセージに変える。
func noteErr(err error, clean string) error {
	switch {
	case vault.IsErrNotFound(err):
		return userErrf("`%s` は存在しません", clean)
	case vault.IsErrAlreadyExists(err):
		return userErrf("`%s` はすでに存在します。append_note で追記するか、別のパスを使ってください", clean)
	case errors.Is(err, vault.ErrHashMismatch):
		return &userError{msg: msgHashMismatch}
	case errors.Is(err, syscall.ENAMETOOLONG):
		return &userError{msg: msgNameTooLong}
	}
	return err
}

// folderErr は List / Search のエラーを Claude 向けのメッセージに変える。
// パスの検証エラーも内部エラーも、詳細（絶対パスなど）はログにだけ出す。
func folderErr(err error, folder string) error {
	if vault.IsErrNotFound(err) {
		return userErrf("フォルダ `%s` は存在しません", folder)
	}
	log.Printf("mcp folder=%q error: %v", folder, err)
	return userErrf("フォルダ `%s` を読めません。Vault ルートからの相対パスを指定してください", folder)
}

func requireText(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return userErrf("%s が空です。内容を指定してください", name)
	}
	return nil
}

// logWrite は書き込み系ツールの結果を1行ログに出す。本文は出さない。
func logWrite(tool, clean string, err error) {
	if clean == "" {
		clean = "(invalid)"
	}
	log.Printf("mcp tool=%s path=%s ok=%t", tool, clean, err == nil)
}

// ---- 読み取り系 ----

func (t *tools) listNotes(_ context.Context, in listNotesIn) (listNotesOut, string, error) {
	notes, err := t.opts.Vault.List(in.Folder, in.Recursive, clampLimit(in.Limit, listDefaultLimit, listMaxLimit))
	if err != nil {
		return listNotesOut{}, "", folderErr(err, in.Folder)
	}
	out := listNotesOut{Notes: make([]noteInfoOut, 0, len(notes))}
	for _, n := range notes {
		out.Notes = append(out.Notes, noteInfoOut{Path: n.Path, Modified: t.formatTime(n.Modified), Size: n.Size})
	}
	return out, "", nil
}

func (t *tools) searchNotes(_ context.Context, in searchNotesIn) (searchNotesOut, string, error) {
	if err := requireText("query", in.Query); err != nil {
		return searchNotesOut{}, "", err
	}
	hits, err := t.opts.Vault.Search(in.Query, in.Folder, clampLimit(in.Limit, searchDefaultLimit, searchMaxLimit))
	if err != nil {
		return searchNotesOut{}, "", folderErr(err, in.Folder)
	}
	out := searchNotesOut{Hits: make([]searchHitOut, 0, len(hits))}
	for _, h := range hits {
		out.Hits = append(out.Hits, searchHitOut{Path: h.Path, Modified: t.formatTime(h.Modified), Excerpt: h.Excerpt})
	}
	return out, "", nil
}

func (t *tools) readNote(_ context.Context, in pathIn) (readNoteOut, string, error) {
	clean, err := t.cleanPath(in.Path)
	if err != nil {
		return readNoteOut{}, "", err
	}
	n, err := t.opts.Vault.ReadRaw(clean)
	if err != nil {
		return readNoteOut{}, "", noteErr(err, clean)
	}
	out := readNoteOut{Path: n.Path, Content: n.Content, Hash: n.Hash, Modified: t.formatTime(n.Modified)}
	if fm, _, err := vault.UnmarshalNote(n.Content); err == nil {
		out.Frontmatter = &fm
	}
	return out, "", nil
}

// ---- 書き込み系 ----

func (t *tools) createNote(_ context.Context, in createNoteIn) (_ pathOut, _ string, err error) {
	var clean string
	defer func() { logWrite("create_note", clean, err) }()
	if clean, err = t.cleanPath(in.Path); err != nil {
		return pathOut{}, "", err
	}
	if err = t.checkWritable(clean); err != nil {
		return pathOut{}, "", err
	}
	if err = checkSafeName(clean); err != nil {
		return pathOut{}, "", err
	}
	existing, err := t.opts.Vault.CaseConflictDir(clean)
	if err != nil {
		return pathOut{}, "", noteErr(err, clean)
	}
	if existing != "" {
		return pathOut{}, "", userErrf("フォルダ `%s` が既にあります。大文字小文字を合わせてください", existing)
	}
	content := vault.MarshalNote(BuildFrontmatter(in.Frontmatter, clean), in.Body)
	if err = t.opts.Vault.CreateRaw(clean, content); err != nil {
		return pathOut{}, "", noteErr(err, clean)
	}
	return pathOut{Path: clean}, "", nil
}

func (t *tools) appendNote(_ context.Context, in appendNoteIn) (_ pathOut, _ string, err error) {
	var clean string
	defer func() { logWrite("append_note", clean, err) }()
	if clean, err = t.cleanPath(in.Path); err != nil {
		return pathOut{}, "", err
	}
	if err = t.checkWritable(clean); err != nil {
		return pathOut{}, "", err
	}
	if err = requireText("content", in.Content); err != nil {
		return pathOut{}, "", err
	}
	heading := strings.TrimSpace(in.Heading)
	if err = t.opts.Vault.AppendRaw(clean, in.Content, heading); err != nil {
		if errors.Is(err, vault.ErrHeadingNotFound) {
			if !strings.HasPrefix(heading, "#") {
				return pathOut{}, "", userErrf("見出し `%s` が見つかりません（見出しは # Memo のように # を含めて指定してください）", heading)
			}
			return pathOut{}, "", userErrf("見出し `%s` が見つかりません", heading)
		}
		return pathOut{}, "", noteErr(err, clean)
	}
	return pathOut{Path: clean}, "", nil
}

func (t *tools) updateNote(_ context.Context, in updateNoteIn) (_ updateNoteOut, _ string, err error) {
	var clean string
	defer func() { logWrite("update_note", clean, err) }()
	if clean, err = t.cleanPath(in.Path); err != nil {
		return updateNoteOut{}, "", err
	}
	if err = t.checkWritable(clean); err != nil {
		return updateNoteOut{}, "", err
	}
	if err = requireText("content", in.Content); err != nil {
		return updateNoteOut{}, "", err
	}
	hash := strings.TrimSpace(in.Hash)
	if hash == "" {
		return updateNoteOut{}, "", userErrf("hash がありません。read_note で得た hash を渡してください")
	}
	// pre-commit フックは created の無いノートを管理しなくなるので、created 行を消す更新は拒否する。
	// 検査するのは hash が一致した内容だけ。ReplaceRaw も同じ hash で照合するので、
	// 検査した内容と置き換える内容は同じになる。
	cur, err := t.opts.Vault.ReadRaw(clean)
	if err != nil {
		return updateNoteOut{}, "", noteErr(err, clean)
	}
	if cur.Hash != hash {
		return updateNoteOut{}, "", &userError{msg: msgHashMismatch}
	}
	if frontmatterHasCreated(cur.Content) && !frontmatterHasCreated(in.Content) {
		return updateNoteOut{}, "", &userError{msg: msgCreatedRemoved}
	}
	if err = t.opts.Vault.ReplaceRaw(clean, in.Content, hash); err != nil {
		return updateNoteOut{}, "", noteErr(err, clean)
	}
	return updateNoteOut{Path: clean, Hash: vault.HashContent(in.Content)}, "", nil
}

func (t *tools) appendDailyMemo(_ context.Context, in appendDailyMemoIn) (_ appendDailyMemoOut, _ string, err error) {
	var clean string
	defer func() { logWrite("append_daily_memo", clean, err) }()
	if err = requireText("content", in.Content); err != nil {
		return appendDailyMemoOut{}, "", err
	}
	loc := t.opts.location()
	var date time.Time
	if d := strings.TrimSpace(in.Date); d != "" {
		date, err = time.ParseInLocation("2006-01-02", d, loc)
		if err != nil {
			return appendDailyMemoOut{}, "", userErrf("date `%s` は YYYY-MM-DD 形式で指定してください（例: 2026-11-01）", in.Date)
		}
	} else {
		date = t.opts.now().In(loc)
	}
	clean = DailyNotePath(date)
	content := in.Content
	created, err := t.opts.Vault.UpsertRaw(clean, RenderDailyNote(date), func(old string) (string, error) {
		return AppendMemo(old, content), nil
	})
	if err != nil {
		return appendDailyMemoOut{}, "", noteErr(err, clean)
	}
	out := appendDailyMemoOut{Path: clean, Created: created}
	if created {
		return out, msgDailyCreated, nil
	}
	return out, "", nil
}
