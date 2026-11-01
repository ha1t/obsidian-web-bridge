# obsidian-web-bridge リモートMCP 設計書

作成日: 2026-11-01

## 目的と背景

Claude Desktop（および claude.ai Web / スマホアプリ）から Obsidian Vault を読み書きできるようにする。
claude.ai の「カスタムコネクタ」としてリモートMCPサーバーを登録し、どの端末の Claude からでも同じ Vault を扱えるようにする。

ユーザーが想定している具体的な使い方:

- Claude Desktop と会話したあと、その会話の内容を Obsidian にノートとして記録させる
- Claude が会話で得た情報（例: 軽トラの車検情報）を Obsidian に記録させる
- 海外サイトを日本語化・要約した結果を Obsidian に保存させる
- DailyReports の最近の日報を読ませて、改善案をまとめさせる
- 今日の DailyNote の Memo 欄に何か書いてもらう

構成は既存の SwitchbotWeb（`~/src/SwitchbotWeb/mcp_server.py`）と同じ方式を取る。
Cloudflare Tunnel + Cloudflare Access（Managed OAuth）で公開し、サーバーは Access が付与する JWT を検証する。
SwitchBot は claude.ai のカスタムコネクタとしてすでにこの方式で動いている。

## ゴール

- claude.ai カスタムコネクタとして登録でき、Claude Desktop / Web / スマホから使える
- ノートの一覧・検索・読み取り・作成・追記・全文更新ができる
- 今日（または指定日）の DailyNote の `# Memo` 見出しに追記できる。DailyNote が無ければテンプレートと同じ形で作ってから追記する
- Cloudflare Access の認証を通らないリクエストはすべて拒否する

## 非ゴール

- ノートの削除（MCPツールとしては提供しない）
- ノートのリネーム・移動
- 添付ファイル（画像など）の読み書き
- 全文検索インデックス（meilisearch 等）の導入。今回は素朴な走査で十分とする
- ローカル stdio MCP（Claude Desktop の `claude_desktop_config.json` 方式）は今回作らない
- git の commit / push。Vault の同期は既存の仕組み（後述）に任せる

## アンチゴール（動いても失敗とみなすもの）

- Access の JWT なしで、インターネットから Vault を読み書きできてしまう
- 既存の LAN 向け HTTP API（`/api/v1/notes`、APIキー認証、`0.0.0.0:18484`）がインターネットに露出する、または挙動が変わる。
  このAPIは他マシンの `claude-daily-report` スキル（Vault の無い端末）が DailyReports を書くのに使っているので壊さない
- サーバーが `created` / `modified` を書いて、Vault の pre-commit フックと二重管理になる
- 追記や見出しへの追記で、既存ノートの frontmatter の並び・書式・本文が書き換わる（既存の `MarshalNote` で再生成すると YAML の順序や書式が変わるので、追記系は生テキストで操作する）
- ユーザーが別端末で編集した内容を、Claude の全文更新が黙って上書きする
- 自動生成フォルダ（DailyReports 等）に書き込んで、生成スクリプトと衝突する

## 現状

- Go 製の単一バイナリ。`server` サブコマンドで HTTP API、`create/read/update/delete` で CLI クライアント
- パッケージ: `vault`（CRUD・パス検証・frontmatter）、`server`（APIキー認証 + ハンドラー）、`config`（YAML）、`client`、`cmd`（cobra）
- `go.mod` は `go 1.22.2`。サーバーの Go も 1.22.2
- systemd ユーザーユニット `obsidian-web-bridge.service` で `server --config config.yaml` として常駐中
- `vault_path` は `/home/ha1t/src/ObsidianGitVault`（Vault 本体）
- Vault の `created` / `modified` は git の pre-commit フック（`~/src/ObsidianGitVault/.githooks/`、`core.hooksPath=.githooks` 設定済み）が管理している。新規追加ファイルには `created`/`modified` を追加し、既存ファイルの変更では `modified` を更新する。書式は `YYYY-MM-DD HH:MM`。cron の `sync-obsidian.sh` の commit でもフックが走る
- PC・スマホの Obsidian は、ユーザーが開かない限り DailyNote を自動作成しない（ユーザー確認済み）
- Vault の同期: cron で毎時30分に `~/src/ObsidianBookmark/sync-obsidian.sh` が commit/push する。ほかの端末の Obsidian も自動 commit/push しているので、他端末への反映は最長1時間程度
- `cloudflared.service`（システム側）が動いている。`/etc/cloudflared/config.yml` の ingress には `earth.project-p.jp → localhost:80` がある。SwitchBot MCP の公開経路はダッシュボード管理の可能性もあり、ここからは確定できない

## 決定事項（ユーザーが決めたもの）

- 方式は「リモートMCP + Cloudflare Access」（ローカル stdio ではない）
- ノートは基本的に `Notes/` に平置きする。関係のある既存ノート・フォルダがあればその近くに置く。会話ログ用のサブフォルダは作らない
- Claude が書いたものは frontmatter のタグで識別できるようにする
- DailyNotes には書き込んでよいが、`# Memo` 欄への追記だけ（全文書き換えはしない。プレイバックで提示し異論なし）
- 指定日の DailyNote が無ければ、テンプレートと同じ形で作ってから追記する
- Claude が作ったものではない既存ノートへの追記・全文書き換えも許可する

## 設計

### プロセス構成

既存バイナリの `server` サブコマンドで、2つのリスナーを同時に立てる。

| リスナー | バインド | 認証 | 用途 |
|---|---|---|---|
| 既存 HTTP API | `bind:port`（既定 `0.0.0.0:18484`） | APIキー（Bearer） | LAN 内 CLI・スキル用。挙動は変えない |
| MCP（新規） | `mcp.bind:mcp.port`（既定 `127.0.0.1:18485`） | Cloudflare Access JWT | cloudflared 経由で公開 |

- MCP リスナーは設定に `mcp` セクションがあるときだけ起動する（無ければ従来どおり）
- MCP は公式 Go SDK `github.com/modelcontextprotocol/go-sdk` の Streamable HTTP ハンドラーで実装する。エンドポイントのパスは `/mcp`
- 両リスナーは同じ `vault.Vault` を共有する
- 片方が起動に失敗したらプロセス全体をエラー終了させる（systemd の `Restart=on-failure` に任せる）

### 設定

`config.yaml` に以下を追加する。

```yaml
mcp:
  bind: "127.0.0.1"
  port: 18485
  cf_access_team_domain: "<team>.cloudflareaccess.com"
  cf_access_aud: "<Application Audience Tag>"
  timezone: "Asia/Tokyo"          # DailyNote の「今日」の判定に使う
  readonly_folders:               # 書き込み禁止フォルダ（Vault ルートからの相対）
    - DailyReports
    - MorningBrief
    - Calendar
    - Garmin
    - MonthlyReview
    - Templates
    - bases
```

- `mcp` セクションがあるのに `cf_access_team_domain` / `cf_access_aud` が空なら起動エラーにする（認証なしで公開される事故を防ぐ）
- `config.yaml.example` にも同じ例を追記する
- `readonly_folders` の初期値は上記のとおり（自動生成・テンプレート・振り返り系）。`DailyNotes` と `Clippings` は書き込み可

### 認証（Cloudflare Access JWT 検証）

SwitchbotWeb の `CloudflareAccessMiddleware` と同等のものを Go で実装する。

- リクエストヘッダー `Cf-Access-Jwt-Assertion` を必須にする。無ければ 403
- 公開鍵は `https://<team_domain>/cdn-cgi/access/certs`（JWKS）から取得し、キャッシュする（`kid` が見つからないときに再取得）
- 検証項目: 署名（RS256）、`aud` が `cf_access_aud` を含む、`iss` が `https://<team_domain>`、`exp` / `nbf`
- 検証失敗はすべて 403。理由はサーバーログにだけ出し、レスポンスには出さない
- MCP SDK 側の Host / Origin 検証（DNS リバインディング対策）は、公開ホスト名で来るため無効化してよい。全リクエストで JWT を必須にしているので、リバインディングの影響は受けない（SwitchBot と同じ判断）
- JWT ライブラリは `github.com/golang-jwt/jwt/v5` 程度の定番を使う。JWKS 取得は自前実装でも `MicahParks/keyfunc` でもよい（builder の判断）

### MCP サーバーの instructions（Claude への説明文）

サーバーの `instructions` に、Vault の使い方の規約を日本語で書く。内容の要点:

- これはユーザー個人の Obsidian Vault。Markdown ノートを読み書きできる
- フォルダの意味: `DailyNotes`（ユーザー本人の日記。Memo 欄への追記だけ行う）、`DailyReports`（Claude Code の作業日報。読み取り専用）、`Notes`（一般のノート。基本はここに平置き）、`Clippings`（Web記事）、`MonthlyReview`・`MorningBrief`・`Calendar`・`Garmin`（自動生成。読み取り専用）
- 新規ノートは基本 `Notes/<タイトル>.md` に平置きする。関係する既存ノート・フォルダがあれば `search_notes` で探してその近くに置く。新しいサブフォルダは作らない
- Web記事の翻訳・要約は `Clippings/<日本語タイトル>.md` に置き、frontmatter の `source` に元URLを入れる
- 書く前に `search_notes` で同じ話題のノートが無いか確認し、あれば新規作成より追記を優先する
- 全文更新は `read_note` で得た `hash` を必ず渡す
- ファイル名に `/ \ : * ? " < > |` は使わない

### frontmatter の規約

Claude が新規作成するノート（`create_note`）には、サーバー側で以下を補完する。
Claude が渡した frontmatter とマージし、Claude 側の値を優先する。ただし `tags` は和集合にする。

```yaml
tags:
  - source/claude-desktop
source: claude-desktop            # Claude が source を渡した場合（元URLなど）はそちらを優先
```

- `Clippings/` 配下に作る場合は `tags` に `clippings` も足す
- **`created` / `modified` はサーバーでは書かない**（Claude が渡しても取り除く）。Vault の pre-commit フックが次のコミット時に付ける（[設計判断] 二重管理を避けるため）
- 既存ノートへの追記・更新では frontmatter に一切触れない

### ツール

すべてのパスは Vault ルートからの相対パス。既存の `ValidatePath`（絶対パス禁止、`..` 禁止、`.md` のみ）を通す。
さらに、シンボリックリンクを解決した実パスが Vault ルート配下であることを確認する。
`.git` / `.obsidian` / `.trash` など `.` 始まりのディレクトリは読み書き・一覧の対象外にする。

| ツール | 入力 | 動作 | 書き込み禁止フォルダ |
|---|---|---|---|
| `list_notes` | `folder`（省略時ルート）, `recursive`（既定 false）, `limit`（既定 30、上限 200） | ノートを更新日時（mtime）の新しい順に返す。各要素は `path`, `modified`, `size` | 対象 |
| `search_notes` | `query`, `folder`（省略可）, `limit`（既定 20、上限 100） | ファイル名と本文の部分一致（大文字小文字を区別しない）。各要素は `path`, `modified`, マッチ箇所前後の短い抜粋（数行） | 対象 |
| `read_note` | `path` | 生テキスト全文と、frontmatter をパースしたもの、`hash`（内容の SHA-256）、`modified` を返す | 対象 |
| `create_note` | `path`, `frontmatter`（任意）, `body` | 新規作成。既存なら失敗（既存パスを伝えて追記を促すエラーメッセージ） | 拒否。`DailyNotes` も拒否 |
| `append_note` | `path`, `content`, `heading`（任意） | `heading` 省略時はノート末尾に追記。指定時はその見出しのセクション末尾（次の同レベル以上の見出しの直前）に追記。見出しが無ければ失敗 | 拒否。`DailyNotes` も拒否 |
| `update_note` | `path`, `content`（frontmatter を含む全文）, `hash` | 現在の内容の hash が一致したときだけ全文置換。不一致なら失敗（「別の場所で編集されたので読み直して」） | 拒否。`DailyNotes` も拒否 |
| `append_daily_memo` | `content`, `date`（`YYYY-MM-DD`、省略時は今日） | `DailyNotes/<date>.md` の `# Memo` セクション末尾に追記。ファイルが無ければテンプレートから作成してから追記 | `DailyNotes` は書き込み可 |

フォルダの権限は3区分になる: 通常（読み書き可）、`readonly_folders`（読み取りのみ）、`DailyNotes`（読み取り + `append_daily_memo` のみ）。`DailyNotes` の扱いは設定ではなくコードで固定してよい（[設計判断]）。

`update_note` の hash は、pre-commit フックがコミット時に `modified` 行を書き換えるため、`read_note` と `update_note` の間に自動 sync が挟まると不一致で失敗する。これは許容する（Claude が読み直せば済む）。[設計判断]

ツールのアノテーション: `list_notes` / `search_notes` / `read_note` は readOnlyHint。書き込み系は destructiveHint を付けない（削除しないため）。ただし `update_note` だけは destructiveHint を付ける。

#### 追記系の書き込み規則

- 追記・更新は生テキストに対して行い、既存の frontmatter や本文の書式を変えない（`modified` もサーバーは更新しない。フックに任せる）
- 追記する内容は、前に空行が1つ入るように整える。末尾は改行で終える
- 書き込みは一時ファイルに書いてから rename する（途中で落ちても半端なファイルを残さない）
- 同一プロセス内の同時書き込みはパス単位のロックで直列化する
- 見出しのマッチは、行頭の `#` の数と見出しテキスト（前後の空白を除く）が一致するもの。最初に見つかったものを使う。コードブロック内の `#` 行は見出しとみなさない

#### DailyNote テンプレート

`Templates/Template_DailyNote.md` は Templater 構文なので、サーバーはそれを読まず、同じ形を Go 側で生成する（テンプレートの変化には手動で追従する。Vault の無い端末でも動くようにするためではなく、Templater 構文を評価できないため）。

```markdown
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

- 日付・曜日・月タグ・前後日リンク・やることリスト名・mobile 埋め込みは対象日から計算する。`created`/`modified` は書かない（テンプレートにはあるが、フックが付けるので省く）
- 「今日」は `mcp.timezone`（既定 `Asia/Tokyo`）で判定する
- `# Memo` は「`# Memo` の行から次の `# ` 見出しの直前まで」。追記はその範囲の末尾（後続見出しの前の空行は1つ残す）
- 既存の DailyNote に `# Memo` 見出しが無い場合は、失敗にせず、`# Smartphone` 見出しがあればその直前に、無ければ末尾に `# Memo` を作って追記する（builder の判断で決めてよいが、ユーザーの本文は消さないこと）
- 衝突リスク: サーバーが作った DailyNote は次の sync（最長1時間）まで他端末に届かない。その間にユーザーが別端末で同じ日の DailyNote を手で開くと add/add コンフリクトになり、`sync-obsidian.sh` が両方残す形で解決して frontmatter が二重になる。他端末は自動作成しないので頻度は低いと判断し、作成は許可する。ただし DailyNote を新規作成したときは、ツールの返り値に「新しく作成しました。他の端末への反映は次の同期（最長1時間）までかかります」と含める。README の手順書にも注意を書く

### エラーの返し方

MCP のツールエラー（`isError: true`）として、Claude が次の行動を判断できる日本語メッセージを返す。例:

- 「`Notes/軽トラ車検.md` はすでに存在します。append_note で追記するか、別のパスを使ってください」
- 「`DailyReports` は読み取り専用フォルダです」
- 「別の場所で編集されています。read_note で読み直してから update_note してください」

### ログ

- 書き込み系ツールの実行（ツール名・パス・成否）を標準出力に1行ずつ出す（journald に残る）。本文は出さない
- JWT 検証失敗も理由付きでログに出す

## デプロイ・運用（手順書として README に書く）

サーバー側（builder が実施）:

1. `config.yaml` に `mcp` セクションを追加
2. ビルドして `systemctl --user restart obsidian-web-bridge`
3. `curl http://127.0.0.1:18485/mcp` が JWT なしで 403 になることを確認

Cloudflare 側（ユーザーがダッシュボードで実施。手順書に SwitchBot と同じ流れで書く）:

1. Tunnel に公開ホスト名を追加し、`http://localhost:18485` に向ける（ホスト名はユーザーが決める）
2. Zero Trust → Access でそのホスト名のアプリを作り、Managed OAuth を有効化、ポリシーは自分のメールのみ許可
3. アプリの AUD タグとチームドメインを `config.yaml` に入れて再起動
4. claude.ai の設定 → コネクタ → カスタムコネクタ追加で `https://<ホスト名>/mcp` を登録

## 制約

- Go が 1.22.2。公式 MCP Go SDK がより新しい Go を要求する場合は、`go.mod` の `toolchain` 指定でツールチェーンを自動取得するか、サーバーの Go を上げる。どちらにするかは builder の判断でよいが、ビルド手順を README に反映すること
- 既存の HTTP API・CLI・テストは壊さない（`go test ./...` が通ること）
- ホスト固有の絶対パスをコードに埋め込まない（パスはすべて設定から）

## テスト

- vault 層: 追記（末尾・見出し）、hash 付き更新の成功・競合、frontmatter が変更されないこと、書き込み禁止フォルダ・DailyNotes への create/append/update の拒否、create_note で created/modified が取り除かれること、`.` 始まりディレクトリの除外、シンボリックリンクで外に出るパスの拒否
- DailyNote: 新規作成時のテンプレート生成（月またぎ・年またぎの前後日リンク、曜日）、既存ノートの Memo 追記、Memo 見出しが無い場合
- 一覧・検索: 更新日順、limit、folder 絞り込み
- 認証: テスト用の鍵で署名した JWT（正常、aud 不一致、iss 不一致、期限切れ、ヘッダー無し）を、JWKS をモックして検証
- MCP: SDK のインメモリ／httptest トランスポートでツール呼び出しを1通り

## builder に任せること

- パッケージの切り方（例: `mcpserver` パッケージ、`vault` への追記系メソッド追加）
- JWT / JWKS ライブラリの選定
- `search_notes` の実装方法（逐次走査で十分。Vault は約 8,000 ノート）と抜粋の長さ
- ツールの入出力スキーマの細部（フィールド名は上記を基本にする）
