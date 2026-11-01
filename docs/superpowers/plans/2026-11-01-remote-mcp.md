# リモートMCP 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** obsidian-web-bridge に Cloudflare Access で保護された Streamable HTTP の MCP エンドポイントを追加し、claude.ai カスタムコネクタから Vault を読み書きできるようにする。

**Architecture:** `vault` パッケージに生テキストのまま操作する読み書き・一覧・検索を足す。新パッケージ `mcpserver` に、フォルダ権限、frontmatter の補完、DailyNote 生成、Cloudflare Access の JWT 検証、MCP ツール定義をまとめる。`server` サブコマンドは、設定に `mcp` セクションがあれば、既存APIのリスナーとは別に `127.0.0.1:18485` で MCP リスナーを立てる。

**Tech Stack:** Go 1.25（toolchain 自動取得）、`github.com/modelcontextprotocol/go-sdk` v1.8.0、`github.com/golang-jwt/jwt/v5` v5.3.1、既存の cobra / yaml.v3

**Spec:** `docs/superpowers/specs/2026-11-01-remote-mcp-design.md`

## Global Constraints

- 既存の HTTP API（`/api/v1/notes`、Bearer APIキー、既定 `0.0.0.0:18484`）と CLI の挙動を変えない。既存テストはすべて通ったままにする
- MCP リスナーの既定値は `127.0.0.1:18485`、パスは `/mcp`
- `mcp` セクションがあるのに `cf_access_team_domain` か `cf_access_aud` が空なら起動エラーにする
- JWT 検証の失敗はすべて HTTP 403。理由はログにだけ出す
- サーバーは `created` / `modified` を一切書かない（pre-commit フックが管理する）。既存ノートの frontmatter と本文の書式は追記・更新で変えない
- `DailyNotes` に書けるのは `append_daily_memo` だけ。`readonly_folders` の既定値は `DailyReports, MorningBrief, Calendar, Garmin, MonthlyReview, Templates, bases`
- `.` で始まるディレクトリ（`.git`、`.obsidian`、`.trash` など）は読み書き・一覧・検索の対象外
- シンボリックリンクを解決した実パスが Vault ルートの外を指す場合は拒否する
- ツールのエラーは `isError: true` と日本語メッセージで返す（Go の error で返すとプロトコルエラーになってしまうため）
- ホスト固有の絶対パスをコードに書かない
- ノートの本文はログに出さない

## Review Focus

1. **Windows 改行（CRLF）のノート**: スマホや Windows で作られたノートに見出し追記・Memo 追記をしても、見出しが見つかり、既存の改行コードを壊さないこと → Task 2 にテストを追加
2. **コードブロック内の `# ` 行**: ` ``` ` 内の `# Memo` のような行を見出しとみなさないこと → Task 2 にテストを追加
3. **Unicode 正規化（NFD）のファイル名**: Mac 由来の濁点分解ファイル名でも、検索のファイル名部分一致でヒットすること → Task 4 にテストを追加（クエリとファイル名の両方を NFC に正規化して比較）
4. **パスの書き方の揺れ**: `./Notes/a.md`、`Notes//a.md`、`notes/...`（大文字小文字違い）、`DailyNotes/../Notes/a.md`。前の2つは正規化して受け付ける。`..` を含むものは拒否する。大文字小文字違いの `dailynotes/` も DailyNotes として扱う（フォルダ判定は大文字小文字を区別しない） → Task 5 にテストを追加
5. **年またぎの DailyNote**: 2026-12-31 と 2027-01-01 の前後日リンク、月タグ、やることリスト名、曜日 → Task 5 にテストを追加

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `config/config.go`（変更） | `MCPConfig` の追加と既定値・検証 |
| `vault/markdown.go`（新規） | 文字列だけを扱う純粋関数: 見出しを探す、セクション末尾に追記する、末尾に追記する |
| `vault/raw.go`（新規） | パス解決（正規化・`.` ディレクトリ・symlink の検査）、生テキストの読み取りと hash、アトミック書き込み、パス単位のロック、CreateRaw / AppendRaw / ReplaceRaw |
| `vault/list.go`（新規） | List（mtime の降順）、Search（ファイル名と本文の部分一致、抜粋付き） |
| `mcpserver/policy.go`（新規） | フォルダの権限区分の判定 |
| `mcpserver/frontmatter.go`（新規） | create_note 用の frontmatter 補完 |
| `mcpserver/daily.go`（新規） | DailyNote の本文生成と Memo への追記 |
| `mcpserver/cfaccess.go`（新規） | JWKS の取得・キャッシュと JWT 検証のミドルウェア |
| `mcpserver/server.go`（新規） | MCP サーバーの組み立て、instructions、Handler |
| `mcpserver/tools.go`（新規） | 7ツールの入出力型とハンドラー |
| `cmd/server.go`（変更） | 2つのリスナーの起動 |
| `config.yaml.example`、`README.md`（変更） | 設定例、デプロイ手順 |

---

### Task 1: MCP 設定の読み込み

**Files:**
- Modify: `config/config.go`
- Test: `config/config_test.go`

**Interfaces:**
- Produces:
  ```go
  type MCPConfig struct {
      Bind               string   `yaml:"bind"`
      Port               int      `yaml:"port"`
      CFAccessTeamDomain string   `yaml:"cf_access_team_domain"`
      CFAccessAUD        string   `yaml:"cf_access_aud"`
      Timezone           string   `yaml:"timezone"`
      ReadonlyFolders    []string `yaml:"readonly_folders"`
  }
  // Config に追加: MCP *MCPConfig `yaml:"mcp"` （セクションが無ければ nil）
  var DefaultReadonlyFolders = []string{"DailyReports", "MorningBrief", "Calendar", "Garmin", "MonthlyReview", "Templates", "bases"}
  ```

- [ ] **Step 1: 失敗するテストを書く**

  - `TestLoad_NoMCPSection`: 既存形式の YAML → `cfg.MCP == nil`
  - `TestLoad_MCPDefaults`: `mcp:` に team_domain と aud だけ → `Bind == "127.0.0.1"`、`Port == 18485`、`Timezone == "Asia/Tokyo"`、`ReadonlyFolders` が `DefaultReadonlyFolders` と等しい
  - `TestLoad_MCPMissingAccess`: `mcp:` があって `cf_access_aud` が空 → エラー。メッセージに `cf_access_aud` を含む（team_domain が空の場合も同様に確認）
  - `TestLoad_MCPInvalidTimezone`: `timezone: "Mars/Base"` → エラー
  - `TestLoad_MCPReadonlyOverride`: `readonly_folders: [Foo]` → `[]string{"Foo"}`（既定値とマージしない）

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test ./config/ -run 'TestLoad_MCP|TestLoad_NoMCP' -v`
  Expected: FAIL（`cfg.MCP undefined`）

- [ ] **Step 3: `config.go` に `MCPConfig` を追加し、`Load` の最後で既定値の補完と検証を行う**

  タイムゾーンは `time.LoadLocation` で検証する。

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./config/ -v`
  Expected: PASS（既存テストも含む）

- [ ] **Step 5: コミット**

  ```bash
  git add config/
  git commit -m "feat: config.yamlにmcpセクションを追加"
  ```

---

### Task 2: Markdown テキストへの追記（純粋関数）

**Files:**
- Create: `vault/markdown.go`
- Test: `vault/markdown_test.go`

**Interfaces:**
- Produces:
  ```go
  var ErrHeadingNotFound = errors.New("heading not found")
  // heading は "# Memo" や "## 作業内容" のように # を含めて渡す
  func AppendToSection(text, heading, content string) (string, error)
  func AppendToEnd(text, content string) string
  func HasHeading(text, heading string) bool
  ```

**規則**（spec「追記系の書き込み規則」より）:
- 見出しの一致: `#` の数が同じで、`#` の後のテキストを TrimSpace したものが一致する、最初の行
- セクションの終わり: 次に来る、同じレベル以上（`#` の数が同じか少ない）の見出しの直前。無ければテキストの末尾
- ` ``` ` または `~~~` で始まる行でフェンスの開閉を切り替え、フェンス内の行は見出しとみなさない
- 挿入する形: セクション本文の末尾にある空行を取り除いてから、`\n` + content（末尾の改行は1つにそろえる）を入れる。後ろに見出しが続く場合は、その前に空行を1つ残す
- 見出しの直後にいきなり追記する場合（セクションが空）は、`見出し\n\ncontent\n` の形にする
- 改行コード: 入力に `\r\n` が含まれていれば、挿入する content の改行も `\r\n` にする

- [ ] **Step 1: 失敗するテストを書く**

  ```go
  func TestAppendToSection_EmptyMemo(t *testing.T) {
      in := "# Diary\n\n日記\n\n# Memo\n\n# Smartphone\n\n![[mobile#Memo]]\n"
      got, err := AppendToSection(in, "# Memo", "メモ1")
      // want:
      want := "# Diary\n\n日記\n\n# Memo\n\nメモ1\n\n# Smartphone\n\n![[mobile#Memo]]\n"
  }
  func TestAppendToSection_ExistingContent(t *testing.T) {
      in := "# Memo\n\n- a\n\n\n# Smartphone\n"
      // AppendToSection(in, "# Memo", "- b") → "# Memo\n\n- a\n\n- b\n\n# Smartphone\n"
  }
  ```
  - `TestAppendToSection_LastSection`: `"# A\n\nx\n# Memo\n\nold"` + `"new"` → `"# A\n\nx\n# Memo\n\nold\n\nnew\n"`
  - `TestAppendToSection_SubheadingStaysInside`: `# Diary` セクション内の `## 見出し` を越えて、次の `# Memo` の直前に入る
  - `TestAppendToSection_NotFound`: `errors.Is(err, ErrHeadingNotFound)`
  - `TestAppendToSection_IgnoresFencedHeading`（Review Focus 2）: ` ```\n# Memo\n``` ` だけを含むテキスト → `ErrHeadingNotFound`。フェンスの後に本物の `# Memo` があれば、そちらに追記される
  - `TestAppendToSection_CRLF`（Review Focus 1）: `"# Memo\r\n\r\n# Smartphone\r\n"` + `"a\nb"` → `"# Memo\r\n\r\na\r\nb\r\n\r\n# Smartphone\r\n"`
  - `TestAppendToEnd`: `"x"` + `"y"` → `"x\n\ny\n"`、`"x\n\n\n"` + `"y"` → `"x\n\ny\n"`、`""` + `"y"` → `"y\n"`
  - `TestHasHeading`: 一致する、レベル違いは一致しない、フェンス内は一致しない

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test ./vault/ -run 'TestAppendTo|TestHasHeading' -v`
  Expected: FAIL（undefined）

- [ ] **Step 3: `vault/markdown.go` を実装する**

  行に分割してから処理する。改行コードは入力から判定し、`\r\n` なら `\r` を外して処理し、最後に戻す。

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./vault/ -v`
  Expected: PASS

- [ ] **Step 5: コミット**

  ```bash
  git add vault/markdown.go vault/markdown_test.go
  git commit -m "feat: Markdownの見出しセクション・末尾への追記関数"
  ```

---

### Task 3: 生テキストの読み書き（パス解決・hash・アトミック書き込み）

**Files:**
- Create: `vault/raw.go`
- Test: `vault/raw_test.go`

**Interfaces:**
- Consumes: Task 2 の `AppendToSection`, `AppendToEnd`
- Produces:
  ```go
  var ErrHashMismatch = errors.New("hash mismatch")
  // 既存の errAlreadyExists / errNotFound と IsErrAlreadyExists / IsErrNotFound を再利用する
  type RawNote struct {
      Path     string    // 正規化済みの相対パス（スラッシュ区切り）
      Content  string
      Hash     string    // SHA-256 の16進表記（小文字）
      Modified time.Time // mtime
  }
  func (v *Vault) CleanPath(notePath string) (string, error)  // 正規化と検証を行い、相対パスを返す
  func (v *Vault) ReadRaw(notePath string) (*RawNote, error)
  func (v *Vault) CreateRaw(notePath, content string) error         // 既存なら errAlreadyExists
  func (v *Vault) AppendRaw(notePath, content, heading string) error // heading が "" なら末尾に追記。見出しが無ければ ErrHeadingNotFound
  func (v *Vault) ReplaceRaw(notePath, content, expectedHash string) error // 不一致なら ErrHashMismatch
  // DailyNote 用: ファイルが無ければ initial で作り、そのあと update を同じロックの中で適用する
  func (v *Vault) UpsertRaw(notePath string, initial string, update func(old string) (string, error)) (created bool, err error)
  func HashContent(content string) string
  ```

**規則:**
- `CleanPath`: `\` を `/` に置換 → `path.Clean` → 先頭の `./` と `/` は不可（`/` 始まりは絶対パスとして拒否）。`..` を要素として含むものは Clean の前の段階で拒否する。`.md` 以外は拒否。どれかの要素が `.` で始まれば拒否
- 実パスの検査: 書き込み時は、存在する一番深い祖先を `filepath.EvalSymlinks` で解決し、Vault ルート（これも EvalSymlinks したもの）の配下であることを確認する。読み取り時はファイル自体を解決して確認する
- アトミック書き込み: 同じディレクトリに `os.CreateTemp(dir, ".owb-*.tmp")` で書いてから `os.Rename`。パーミッションは既存ファイルがあればそれを引き継ぎ、無ければ 0644
- ロック: `Vault` に `sync.Mutex` で守った `map[string]*sync.Mutex` を持たせ、正規化パスごとにロックする
- 既存の `Create/Read/Update/Delete` と `ValidatePath` は変更しない

- [ ] **Step 1: 失敗するテストを書く**

  - `TestCleanPath`: テーブルテスト。`"./Notes/a.md"→"Notes/a.md"`、`"Notes//a.md"→"Notes/a.md"`、`"Notes\\a.md"→"Notes/a.md"`、拒否: `"../a.md"`、`"Notes/../../a.md"`、`"DailyNotes/../Notes/a.md"`、`"/etc/a.md"`、`".obsidian/a.md"`、`"Notes/.trash/a.md"`、`"Notes/a.txt"`、`""`
  - `TestReadRaw_HashAndContent`: 書いた内容がそのまま返り、`Hash == HashContent(content)`
  - `TestReadRaw_SymlinkOutside`: Vault の外のファイルへの symlink `Notes/out.md` → エラー
  - `TestCreateRaw_Exists`: 2回目は `IsErrAlreadyExists`
  - `TestCreateRaw_SymlinkDirOutside`: Vault の外を指すディレクトリへの symlink の下に作ろうとするとエラー
  - `TestCreateRaw_MakesDirs`: `Notes/sub/a.md` が作られる
  - `TestAppendRaw_PreservesFrontmatter`: `"---\nb: 1\na: 2\nmodified: 2026-01-01 10:00\n---\n\n本文"` に追記 → frontmatter 部分がバイト単位で同じ
  - `TestAppendRaw_Heading` / `TestAppendRaw_HeadingNotFound` / `TestAppendRaw_NotFound`（ファイルが無ければ `IsErrNotFound`）
  - `TestReplaceRaw_HashMatch` / `TestReplaceRaw_HashMismatch`（`errors.Is(err, ErrHashMismatch)`、ファイルは変わらない）
  - `TestUpsertRaw_CreatesThenUpdates`: 無いとき `created == true` で、内容は `update(initial)` の結果。あるときは `created == false`
  - `TestWrite_NoTempLeftover`: 書き込み後に `.owb-*.tmp` が残っていない
  - `TestAppendRaw_Concurrent`: 20個の goroutine で同じファイルに追記 → 20行すべてが残る

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test ./vault/ -run 'TestCleanPath|Raw|TestWrite' -v`
  Expected: FAIL

- [ ] **Step 3: `vault/raw.go` を実装する**（`Vault` 構造体にロック用のフィールドを追加。`New` のシグネチャは変えない）

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./vault/ -race -v`
  Expected: PASS

- [ ] **Step 5: コミット**

  ```bash
  git add vault/
  git commit -m "feat: vaultに生テキストの読み書き・hash付き更新・アトミック書き込みを追加"
  ```

---

### Task 4: 一覧と検索

**Files:**
- Create: `vault/list.go`
- Test: `vault/list_test.go`

**Interfaces:**
- Consumes: Task 3 の `CleanPath` の規則（フォルダ版として、`.md` の検査を外した `cleanFolder` を内部で用意する）
- Produces:
  ```go
  type NoteInfo struct {
      Path     string
      Modified time.Time
      Size     int64
  }
  type SearchHit struct {
      Path     string
      Modified time.Time
      Excerpt  string // ファイル名だけが一致したときは ""
  }
  func (v *Vault) List(folder string, recursive bool, limit int) ([]NoteInfo, error)
  func (v *Vault) Search(query, folder string, limit int) ([]SearchHit, error)
  ```

**規則:**
- `folder == ""` は Vault ルート。存在しないフォルダは `errNotFound`
- `.` で始まるディレクトリは降りない。シンボリックリンクのディレクトリも降りない
- List: `.md` だけを対象にし、mtime の降順で並べる。同時刻ならパスの昇順。`limit` を超えた分は切る
- Search: 常に再帰的に探す。一致の判定は、クエリとファイル名・本文の両方を NFC に正規化（`golang.org/x/text/unicode/norm`）し、`strings.ToLower` してから部分一致を見る。ファイル名か本文のどちらかに一致すればヒット。並びは mtime の降順
- 抜粋: 本文で最初に一致した行と、その前後1行（最大3行）。1行が 200 rune を超える場合は、一致位置の周辺 200 rune に切る
- 1MB を超えるファイルは本文検索の対象外にする（ファイル名の一致は見る）

- [ ] **Step 1: 失敗するテストを書く**

  - `TestList_SortedByMtime`: `os.Chtimes` で mtime を3段階に設定 → 新しい順に返る
  - `TestList_NonRecursiveAndRecursive`: サブフォルダのノートは `recursive=false` では返らず、`true` では返る
  - `TestList_Limit`、`TestList_SkipsDotDirs`（`.obsidian/x.md` が返らない）、`TestList_NotFound`
  - `TestSearch_BodyAndFilename`: 本文の一致に抜粋が付き、ファイル名だけの一致は `Excerpt == ""`
  - `TestSearch_CaseInsensitive`: `"Claude"` で `"claude"` を含むノートがヒットする
  - `TestSearch_NFDFilename`（Review Focus 3）: NFD で作ったファイル名 `"ガ"`（`"ガ"`）が、NFC のクエリ `"ガ"` でヒットする
  - `TestSearch_FolderFilter`、`TestSearch_ExcerptLines`（一致行と前後1行）

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test ./vault/ -run 'TestList|TestSearch' -v`
  Expected: FAIL

- [ ] **Step 3: `vault/list.go` を実装する**（`filepath.WalkDir` を使う。`go get golang.org/x/text`）

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./vault/ -v`
  Expected: PASS

- [ ] **Step 5: コミット**

  ```bash
  git add vault/ go.mod go.sum
  git commit -m "feat: vaultにノート一覧（更新日順）と検索を追加"
  ```

---

### Task 5: フォルダ権限・frontmatter 補完・DailyNote 生成

**Files:**
- Create: `mcpserver/policy.go`, `mcpserver/frontmatter.go`, `mcpserver/daily.go`
- Test: `mcpserver/policy_test.go`, `mcpserver/frontmatter_test.go`, `mcpserver/daily_test.go`

**Interfaces:**
- Consumes: Task 3 の `CleanPath` の結果（正規化済みの相対パス）
- Produces:
  ```go
  type FolderClass int
  const (
      FolderWritable FolderClass = iota
      FolderReadonly
      FolderDailyNotes
  )
  type Policy struct{ readonly []string }
  func NewPolicy(readonlyFolders []string) *Policy
  // cleanPath は正規化済み。先頭の要素を大文字小文字を区別せずに比較する
  func (p *Policy) Classify(cleanPath string) FolderClass

  // 渡された frontmatter（nil 可）に tags: source/claude-desktop と source: claude-desktop を補う。
  // tags は和集合で、順序は「渡されたもの → 足したもの」。Clippings 配下なら clippings も足す。
  // created / modified は（大文字小文字を区別せずに）取り除く。tags が文字列で渡された場合は1要素の配列とみなす。
  func BuildFrontmatter(given map[string]any, cleanPath string) map[string]any

  const DailyNotesDir = "DailyNotes"
  func DailyNotePath(date time.Time) string        // "DailyNotes/2026-11-01.md"
  func RenderDailyNote(date time.Time) string      // spec のテンプレート（created/modified の行は無し）
  // "# Memo" に追記する。無ければ "# Smartphone" の直前、それも無ければ末尾に "# Memo" を作ってから追記する
  func AppendMemo(text, content string) string
  ```

**RenderDailyNote の出力**（2026-11-01 の場合。spec の例から `created`/`modified` を除いたもの）:
```
---
title: DailyNote 2026-11-01
tags:
  - 2026/11
  - daily
date: 2026-11-01
day_of_the_week: 日曜日
location: hokkaido
weather:
temp_min:
temp_max:
---

[[2026-10-31]] ::: [[2026-11-02]]

# Todo

![[2026年11月にやることリスト]]

# Diary


# Memo

# Smartphone

![[mobile-2026-11-01#Memo]]
```
やることリストの月はゼロ埋めする（Vault 内の実例が `2026年01月にやることリスト`）。曜日は `日月火水木金土` + `曜日`。

- [ ] **Step 1: 失敗するテストを書く**

  - `TestClassify`（Review Focus 4）: `DailyNotes/2026-11-01.md`→DailyNotes、`dailynotes/x.md`→DailyNotes、`DailyReports/x.md`→Readonly、`dailyreports/x.md`→Readonly、`Notes/x.md`→Writable、`Clippings/x.md`→Writable、`DailyReportsArchive/x.md`→Writable（先頭の要素が完全一致するときだけ）、ルート直下の `x.md`→Writable
  - `TestBuildFrontmatter_Defaults`: nil → `tags == []any{"source/claude-desktop"}`、`source == "claude-desktop"`
  - `TestBuildFrontmatter_MergeTags`: `tags: ["car"]` → `["car","source/claude-desktop"]`。すでに `source/claude-desktop` を含んでいれば重複させない
  - `TestBuildFrontmatter_SourceURLWins`: `source: "https://example.com"` はそのまま残る
  - `TestBuildFrontmatter_Clippings`: `Clippings/a.md` → tags に `clippings` が入る
  - `TestBuildFrontmatter_StripsDates`: `created` / `Modified` を渡しても結果に無い
  - `TestRenderDailyNote_Golden`: 上の出力と完全一致（2026-11-01 JST）
  - `TestRenderDailyNote_YearBoundary`（Review Focus 5）: 2026-12-31 → `[[2026-12-30]] ::: [[2027-01-01]]`、`2026/12`、`2026年12月にやることリスト`、`木曜日`。2027-01-01 → `[[2026-12-31]] ::: [[2027-01-02]]`、`2027/01`、`2027年01月にやることリスト`、`金曜日`
  - `TestAppendMemo_ToTemplate`: `RenderDailyNote` の結果に追記 → `# Memo\n\nメモ\n\n# Smartphone` の形になる
  - `TestAppendMemo_NoMemoHeading`: `# Smartphone` があればその直前に `# Memo\n\nメモ\n\n` が入る。無ければ末尾に `# Memo\n\nメモ\n` が入る。どちらの場合も、元の Diary の内容が残る

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test ./mcpserver/ -v`
  Expected: FAIL

- [ ] **Step 3: 3つのファイルを実装する**（`AppendMemo` は Task 2 の `HasHeading` / `AppendToSection` を使う）

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./mcpserver/ -v`
  Expected: PASS

- [ ] **Step 5: コミット**

  ```bash
  git add mcpserver/
  git commit -m "feat: フォルダ権限・frontmatter補完・DailyNote生成"
  ```

---

### Task 6: Cloudflare Access の JWT 検証ミドルウェア

**Files:**
- Create: `mcpserver/cfaccess.go`
- Test: `mcpserver/cfaccess_test.go`

**Interfaces:**
- Produces:
  ```go
  type AccessVerifier struct { /* teamDomain, aud, certsURL, http.Client, キャッシュした鍵(kid→*rsa.PublicKey), mutex */ }
  // certsURL は通常 "https://<teamDomain>/cdn-cgi/access/certs"。テストでは httptest のURLを渡す
  func NewAccessVerifier(teamDomain, aud, certsURL string, client *http.Client) *AccessVerifier
  func (a *AccessVerifier) Verify(ctx context.Context, token string) error
  func (a *AccessVerifier) Middleware(next http.Handler) http.Handler
  func CertsURL(teamDomain string) string
  ```

**規則:**
- ヘッダー `Cf-Access-Jwt-Assertion` が無い、または検証に失敗したら 403 を返し、本文は `forbidden` だけにする。`log.Printf` で理由を出す
- `jwt/v5` の `jwt.Parse` に `jwt.WithValidMethods([]string{"RS256"})`、`jwt.WithAudience(aud)`、`jwt.WithIssuer("https://"+teamDomain)`、`jwt.WithExpirationRequired()` を渡す
- JWKS の形は `{"keys":[{"kid","kty":"RSA","n","e",...}]}`（base64url）。`kid` がキャッシュに無ければ一度だけ取り直す。取り直しは最短でも 1 分の間隔をあける（不正な kid で JWKS を連打させないため）
- `teamDomain` に `https://` が付いていても付いていなくても受け付ける（内部で取り除く）

- [ ] **Step 1: 失敗するテストを書く**

  テスト内で `rsa.GenerateKey` を作り、JWKS を返す httptest サーバーを立てる。

  - `TestMiddleware_ValidToken`: 正しい aud / iss / exp → 次のハンドラーまで届いて 200
  - `TestMiddleware_MissingHeader` / `WrongAudience` / `WrongIssuer` / `Expired` / `NoExp` / `HS256Token`（`alg` が違う） / `UnknownKid`（別の鍵で署名） → すべて 403、本文は `forbidden`
  - `TestVerifier_RefetchOnUnknownKid`: 最初の JWKS は鍵 A だけ、2回目の取得から鍵 B を返す → 鍵 B のトークンが通る。JWKS サーバーへのリクエストは2回
  - `TestVerifier_RefetchRateLimited`: 未知の kid を短時間に3回 → JWKS へのリクエストは最初の取得 + 1回まで

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test ./mcpserver/ -run 'TestMiddleware|TestVerifier' -v`
  Expected: FAIL

- [ ] **Step 3: 依存を追加して実装する**

  Run: `go get github.com/golang-jwt/jwt/v5@v5.3.1`

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./mcpserver/ -race -v`
  Expected: PASS

- [ ] **Step 5: コミット**

  ```bash
  git add mcpserver/ go.mod go.sum
  git commit -m "feat: Cloudflare AccessのJWT検証ミドルウェア"
  ```

---

### Task 7: MCP サーバーと7つのツール

**Files:**
- Create: `mcpserver/server.go`, `mcpserver/tools.go`
- Test: `mcpserver/tools_test.go`
- Modify: `go.mod`（Go 1.25 と go-sdk）

**Interfaces:**
- Consumes: Task 3・4 の `Vault` メソッド、Task 5 の `Policy` / `BuildFrontmatter` / `DailyNotePath` / `RenderDailyNote` / `AppendMemo`、既存の `vault.MarshalNote` / `vault.UnmarshalNote`
- Produces:
  ```go
  type Options struct {
      Vault    *vault.Vault
      Policy   *Policy
      Location *time.Location
      Now      func() time.Time // テスト用。nil なら time.Now
  }
  func NewMCPServer(opts Options) *mcp.Server
  // NewStreamableHTTPHandler(…, &mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true}) を verifier.Middleware で包み、"/mcp" に載せた mux を返す
  func NewHTTPHandler(opts Options, verifier *AccessVerifier) http.Handler
  ```

**ツールの入出力**（フィールド名は JSON タグ。説明文は日本語で書く）:

| ツール | 入力 | 出力（構造化） | アノテーション |
|---|---|---|---|
| `list_notes` | `folder?`, `recursive?`, `limit?`（既定 30、上限 200） | `{notes: [{path, modified, size}]}` | ReadOnly |
| `search_notes` | `query`, `folder?`, `limit?`（既定 20、上限 100） | `{hits: [{path, modified, excerpt}]}` | ReadOnly |
| `read_note` | `path` | `{path, content, frontmatter, hash, modified}` | ReadOnly |
| `create_note` | `path`, `frontmatter?`, `body` | `{path}` | — |
| `append_note` | `path`, `content`, `heading?` | `{path}` | — |
| `update_note` | `path`, `content`, `hash` | `{path, hash}`（新しい hash） | Destructive |
| `append_daily_memo` | `content`, `date?`（`YYYY-MM-DD`） | `{path, created}` | — |

- `modified` は `opts.Location` で RFC3339 にして返す。`limit` が 0 なら既定値、上限を超えたら上限に丸める
- `read_note` の `frontmatter` は `UnmarshalNote` の結果。パースに失敗したら `null` にして `content` だけ返す（エラーにはしない）
- `create_note` の中身は `MarshalNote(BuildFrontmatter(given, path), body)`
- 権限: 書き込み系の4ツールは、`Classify` の結果が Readonly なら「`<フォルダ>` は読み取り専用フォルダです」、DailyNotes なら「DailyNotes には append_daily_memo で Memo 欄に追記してください」を返す
- `append_daily_memo`: `date` が無ければ `Now().In(Location)` の日付を使う。形式が不正ならエラーメッセージを返す。`UpsertRaw(DailyNotePath(d), RenderDailyNote(d), AppendMemo)` を呼ぶ。作成したときは、テキストの出力に「新しく作成しました。他の端末への反映は次の同期（最長1時間）までかかります」を含める
- エラー文言（spec の例を使う）:
  - 既存: 「`<path>` はすでに存在します。append_note で追記するか、別のパスを使ってください」
  - hash 不一致: 「別の場所で編集されています。read_note で読み直してから update_note してください」
  - 見出しが無い: 「見出し `<heading>` が見つかりません」
  - ノートが無い: 「`<path>` は存在しません」
- ログ: 書き込み系のツールは、成功・失敗ごとに `log.Printf("mcp tool=%s path=%s ok=%t", ...)` を1行出す。本文は出さない
- `NewMCPServer` の `ServerOptions.Instructions` には、spec の「MCP サーバーの instructions」節の箇条書きを日本語の文章にしたものを入れる。ただし `DailyNotes` については「Memo 欄への追記は append_daily_memo を使う」と書き、読み取り専用フォルダの一覧は `Policy` から生成する

- [ ] **Step 1: Go 1.25 と go-sdk を入れる**

  Run: `go get github.com/modelcontextprotocol/go-sdk@v1.8.0 && go mod tidy && go version && go test ./...`
  Expected: `go.mod` の `go` 行が `1.25.0` 以上になる（GOTOOLCHAIN=auto で自動取得される）。既存テストが PASS する。自動取得できない環境の場合は、ここで止めて報告する

- [ ] **Step 2: 失敗するテストを書く**

  `mcp.NewInMemoryTransports()` でクライアントとサーバーをつなぐヘルパー `newTestSession(t, now time.Time) (*mcp.ClientSession, string /*vault dir*/)` を作り、`session.CallTool` で呼ぶ。

  - `TestListTools`: 7つのツール名がそろっていて、`read_note` が ReadOnly、`update_note` が Destructive
  - `TestCreateAndRead`: `create_note` の結果ファイルに `source/claude-desktop` が入り、`created` が無い。`read_note` の hash が `vault.HashContent(ファイルの内容)` と一致する
  - `TestCreateNote_Exists`: `IsError == true` で、文言に「append_note」を含む
  - `TestWriteTools_ReadonlyFolder`: `create_note` / `append_note` / `update_note` を `DailyReports/x.md` に対して呼ぶ → `IsError`、「読み取り専用」を含む
  - `TestWriteTools_DailyNotes`: 同じ3ツールを `DailyNotes/2026-11-01.md` に対して呼ぶ → `IsError`、「append_daily_memo」を含む
  - `TestUpdateNote_HashMismatch`: 文言に「read_note で読み直して」を含み、ファイルは変わらない
  - `TestAppendDailyMemo_CreatesToday`: `now` = 2026-11-01 23:30 JST（UTC では 14:30） → `DailyNotes/2026-11-01.md` が作られ、`# Memo` の下に内容があり、`created == true`、テキストに「最長1時間」を含む
  - `TestAppendDailyMemo_TimezoneBoundary`: `now` = 2026-11-01 15:30 UTC（JST では 11-02 00:30） → `2026-11-02.md`
  - `TestAppendDailyMemo_Existing`: 既存ファイルの Diary の内容が残り、`created == false`
  - `TestAppendDailyMemo_BadDate`: `"2026/11/01"` → `IsError`
  - `TestListAndSearch`: 構造化出力に path と modified（RFC3339、`+09:00`）が入る。limit=1000 を渡しても 200 件までに丸められる（テスト用に 201 件作る）
  - `TestHTTPHandler_RequiresJWT`: `httptest` で `NewHTTPHandler` を立て、ヘッダー無しで POST `/mcp` → 403。Task 6 のテスト用の鍵と JWKS ヘルパーで作った正しいトークンを付け、`Host: obsidian.example.com` で `initialize` を POST → 200（非 localhost の Host でも 403 にならない）

- [ ] **Step 3: テストが失敗することを確認する**

  Run: `go test ./mcpserver/ -run 'TestListTools|TestCreate|TestWriteTools|TestUpdate|TestAppendDaily|TestListAndSearch|TestHTTPHandler' -v`
  Expected: FAIL

- [ ] **Step 4: `server.go` と `tools.go` を実装する**

  ツールハンドラーは `mcp.AddTool[In, Out]` で登録する。業務上のエラーは `&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}` で返し、Go の error は nil にする。

- [ ] **Step 5: テストが通ることを確認する**

  Run: `go test ./... -race`
  Expected: PASS

- [ ] **Step 6: コミット**

  ```bash
  git add mcpserver/ go.mod go.sum
  git commit -m "feat: MCPサーバーと7つのツール（Streamable HTTP, stateless）"
  ```

---

### Task 8: server サブコマンドへの組み込み・ドキュメント・デプロイ

**Files:**
- Modify: `cmd/server.go`, `config.yaml.example`, `README.md`, `config.yaml`（ローカルのみ。git 管理外であることを `git check-ignore config.yaml` で確認する）
- Test: `integration_test.go`（MCP 部分を追加）

**Interfaces:**
- Consumes: Task 1 の `cfg.MCP`、Task 6 の `NewAccessVerifier` / `CertsURL`、Task 7 の `NewHTTPHandler`、`Options`

- [ ] **Step 1: 失敗するテストを書く**

  `TestIntegration_MCPListenerRequiresJWT`: 2つのリスナーを起動する関数 `cmd` 側の `runServers(ctx, cfg) error`（新規。`ctx` が cancel されたら両方を Shutdown する）を空きポートで起動し、次を確認する。
  - 既存API: Bearer キー付きの GET が従来どおり動く
  - MCP: JWT 無しの POST `/mcp` → 403
  - `cfg.MCP == nil` のときは MCP のポートが listen されていない

- [ ] **Step 2: テストが失敗することを確認する**

  Run: `go test . -run TestIntegration_MCP -v`
  Expected: FAIL

- [ ] **Step 3: `cmd/server.go` を `runServers` を使う形に書き換える**

  `cfg.MCP != nil` のときだけ MCP の `http.Server` を立てる。片方の `ListenAndServe` がエラーになったら、もう片方を Shutdown してエラーを返す（`errgroup` は使わず、チャネルで十分）。`SIGINT` / `SIGTERM` で両方を Shutdown する。起動時に `Starting MCP server on 127.0.0.1:18485 (/mcp)` を出す。`runServers` はテストから呼べるように、`cmd` パッケージから export するか、別の小さなパッケージに置く（置き場所は実装者が決める）。

- [ ] **Step 4: テストが通ることを確認する**

  Run: `go test ./... -race`
  Expected: PASS

- [ ] **Step 5: `config.yaml.example` と `README.md` を更新する**

  - example: spec の「設定」節の `mcp:` ブロックをそのまま追加する（値はプレースホルダー）
  - README に「リモートMCP（claude.ai カスタムコネクタ）」の節を追加する。内容は、ビルド（Go 1.25 以上。古い Go でも GOTOOLCHAIN=auto なら自動取得される）、spec の「デプロイ・運用」のサーバー側3手順と Cloudflare 側4手順、ツール一覧、DailyNote の同期衝突の注意（「サーバーが作った DailyNote が同期されるまでの最長1時間、他の端末で同じ日の DailyNote を開かない」）

- [ ] **Step 6: コミット**

  ```bash
  git add cmd/ integration_test.go config.yaml.example README.md
  git commit -m "feat: serverサブコマンドでMCPリスナーを起動、README・設定例を更新"
  ```

- [ ] **Step 7: 本番に反映する（ユーザーの確認を取ってから）**

  1. `config.yaml` に `mcp:` を追加する。team_domain と aud はユーザーに聞く。まだ無ければ、Cloudflare 側の作業が終わるまでこのステップで止める
  2. `go build -o obsidian-web-bridge . && systemctl --user restart obsidian-web-bridge && systemctl --user status obsidian-web-bridge --no-pager`
     Expected: `active (running)`。journal に `Starting MCP server on 127.0.0.1:18485`
  3. `curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18485/mcp`
     Expected: `403`
  4. 既存APIの疎通: `./obsidian-web-bridge read --path "DailyReports/<今日>.md"` が従来どおり動く
  5. Cloudflare 側の4手順をユーザーに案内し、claude.ai でコネクタを登録して `list_notes` が呼べることを確かめてもらう
