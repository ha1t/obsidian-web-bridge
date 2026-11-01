package mcpserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/ha1t/obsidian-web-bridge/vault"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "obsidian-web-bridge"
	serverVersion = "0.2.0"
)

// Options は MCP サーバーの依存をまとめたもの。
type Options struct {
	Vault    *vault.Vault
	Policy   *Policy
	Location *time.Location   // 「今日」の判定と modified の表示に使う。nil なら time.Local
	Now      func() time.Time // テスト用。nil なら time.Now
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o Options) location() *time.Location {
	if o.Location != nil {
		return o.Location
	}
	return time.Local
}

// NewMCPServer は Vault を読み書きする7つのツールを登録した MCP サーバーを返す。
func NewMCPServer(opts Options) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		&mcp.ServerOptions{Instructions: buildInstructions(opts.Policy)},
	)
	registerTools(s, &tools{opts: opts})
	return s
}

// NewHTTPHandler は stateless な Streamable HTTP の MCP エンドポイントを "/mcp" に載せ、
// 全体を Cloudflare Access の JWT 検証で包んだハンドラーを返す。
//
// cloudflared 経由のリクエストは公開ホスト名の Host を持つため、SDK の localhost 保護は外す。
// これが安全なのは、JWT 検証がすべてのリクエストの前に入っているからなので、
// verifier.Middleware を外したり、その外側にハンドラーを足したりしないこと。
func NewHTTPHandler(opts Options, verifier *AccessVerifier) http.Handler {
	srv := NewMCPServer(opts)
	h := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true},
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return verifier.Middleware(mux)
}

func buildInstructions(p *Policy) string {
	readonly := "なし"
	if p != nil {
		if ro := p.ReadonlyFolders(); len(ro) > 0 {
			readonly = strings.Join(ro, "、")
		}
	}
	return `これはユーザー個人の Obsidian Vault です。Markdown ノートを読み書きできます。パスはすべて Vault ルートからの相対パスで、拡張子は .md です。

フォルダの意味:
- ` + DailyNotesDir + `: ユーザー本人の日記です。Memo 欄への追記は append_daily_memo を使ってください（ほかのツールでは書き込めません）。
- DailyReports: Claude Code の作業日報です。
- Notes: 一般のノートです。基本はここに平置きします。
- Clippings: Web 記事の翻訳・要約を置きます。
- MonthlyReview・MorningBrief・Calendar・Garmin: 自動生成されるノートです。

読み取り専用フォルダ（読めますが書き込めません）: ` + readonly + `

書き方の規約:
- 書く前に search_notes で同じ話題のノートが無いか確認し、あれば新規作成より append_note での追記を優先してください。
- 新規ノートは基本 Notes/<タイトル>.md に平置きします。関係する既存のノートやフォルダがあれば search_notes で探して、その近くに置いてください。新しいサブフォルダは作らないでください。
- Web 記事の翻訳・要約は Clippings/<日本語タイトル>.md に置き、frontmatter の source に元の URL を入れてください。
- update_note で全文を置き換えるときは、read_note で得た hash を必ず渡してください。
- ファイル名に / \ : * ? " < > | は使わず、末尾をドットや空白にしないでください。CON・NUL などの Windows の予約名も使えません。
- 追記する内容にトップレベルの見出し（# ）は入れず、見出しは ## 以下を使ってください。
- frontmatter の created / modified は Vault 側で自動で付くので、指定しないでください。`
}
