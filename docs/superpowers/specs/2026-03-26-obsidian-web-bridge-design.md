# Obsidian Web Bridge 設計書

## 概要

Obsidian vaultのMarkdownファイルをHTTP API経由でCRUD操作できるGoアプリケーション。
単一バイナリにAPIサーバーとCLIクライアントを同居させる。

主なユースケース: 複数端末で動作するClaude Codeから日報等のノートをObsidian vaultに書き込む。

## 技術スタック

- 言語: Go
- CLIフレームワーク: cobra
- HTTPサーバー: 標準ライブラリ net/http
- 設定ファイル: YAML (gopkg.in/yaml.v3)
- frontmatterパース: gopkg.in/yaml.v3

## アーキテクチャ

```
obsidian-web-bridge (単一バイナリ)
├── server サブコマンド ... REST APIサーバー
├── create サブコマンド ... ノート作成 (APIクライアント)
├── read   サブコマンド ... ノート取得
├── update サブコマンド ... ノート更新
└── delete サブコマンド ... ノート削除
```

サーバーは vault のファイルシステムを直接操作する。CLIは HTTP クライアントとしてサーバーにリクエストを送る。

## API設計

### 認証

`Authorization: Bearer <API_KEY>` ヘッダーで認証。不正な場合は `401 Unauthorized`。

### エンドポイント

ベースURL: `http://<host>:<port>/api/v1`

| メソッド | パス | 説明 |
|---------|------|------|
| POST | /notes | ノート作成。既に存在すれば 409 Conflict |
| GET | /notes?path=<path> | ノート取得 |
| PUT | /notes | ノート更新。存在しなければ 404 |
| DELETE | /notes | ノート削除。存在しなければ 404 |

### リクエスト/レスポンス

#### POST /notes (作成)

Request:
```json
{
  "path": "DailyReports/2026-03-26-server1.md",
  "frontmatter": {
    "title": "日報 2026-03-26 server1",
    "created": "2026-03-26 18:00",
    "tags": ["daily-report"]
  },
  "body": "## 作業内容\n- ..."
}
```

Response 201:
```json
{
  "path": "DailyReports/2026-03-26-server1.md",
  "created": true
}
```

Response 409 (既に存在):
```json
{
  "error": "file already exists",
  "path": "DailyReports/2026-03-26-server1.md"
}
```

#### GET /notes?path=... (取得)

Response 200:
```json
{
  "path": "DailyReports/2026-03-26-server1.md",
  "frontmatter": {
    "title": "日報 2026-03-26 server1",
    "created": "2026-03-26 18:00",
    "tags": ["daily-report"]
  },
  "body": "## 作業内容\n- ..."
}
```

#### PUT /notes (更新)

Request:
```json
{
  "path": "DailyReports/2026-03-26-server1.md",
  "frontmatter": {
    "title": "日報 2026-03-26 server1 (更新)",
    "created": "2026-03-26 18:00",
    "tags": ["daily-report"]
  },
  "body": "## 作業内容\n- 更新後の内容..."
}
```

Response 200:
```json
{
  "path": "DailyReports/2026-03-26-server1.md",
  "updated": true
}
```

#### DELETE /notes (削除)

Request:
```json
{
  "path": "DailyReports/2026-03-26-server1.md"
}
```

Response 200:
```json
{
  "path": "DailyReports/2026-03-26-server1.md",
  "deleted": true
}
```

### パスバリデーション

- vault rootからの相対パスのみ受け付ける
- `..` を含むパス、絶対パスは 400 Bad Request で拒否（パストラバーサル防止）
- パスは `.md` 拡張子のみ許可

## 設定

### サーバー設定ファイル (config.yaml)

```yaml
vault_path: /home/ha1t/src/ObsidianGitVault
port: 18484
api_key: "your-api-key-here"
bind: "0.0.0.0"
```

### CLI設定

接続先は以下の優先順位で決定:

1. コマンドラインフラグ (`--server`, `--api-key`)
2. 環境変数 (`OWB_SERVER`, `OWB_API_KEY`)

## CLIインターフェース

### サーバー起動

```bash
obsidian-web-bridge server --config config.yaml
obsidian-web-bridge server --vault /path/to/vault --port 18484 --api-key <key>
```

### ノート操作

```bash
# 作成 (bodyを引数で)
obsidian-web-bridge create --path "DailyReports/2026-03-26.md" \
  --title "日報" --tag daily-report --body "内容..."

# 作成 (標準入力から)
echo "長い内容..." | obsidian-web-bridge create --path "Notes/memo.md"

# 作成 (ファイルから)
obsidian-web-bridge create --path "Notes/memo.md" --file ./report.md

# 取得
obsidian-web-bridge read --path "DailyReports/2026-03-26.md"

# 更新
cat updated.md | obsidian-web-bridge update --path "DailyReports/2026-03-26.md"

# 削除
obsidian-web-bridge delete --path "DailyReports/2026-03-26.md"
```

body入力の優先順位: `--file` > stdin > `--body`

## プロジェクト構成

```
obsidian-web-bridge/
├── main.go
├── go.mod
├── cmd/
│   ├── root.go        # cobra root + 共通フラグ
│   ├── server.go      # serverサブコマンド
│   ├── create.go      # createサブコマンド
│   ├── read.go        # readサブコマンド
│   ├── update.go      # updateサブコマンド
│   └── delete.go      # deleteサブコマンド
├── server/
│   ├── handler.go     # HTTPハンドラー
│   ├── middleware.go   # 認証ミドルウェア
│   └── server.go      # サーバー起動・ルーティング
├── vault/
│   ├── vault.go       # vault操作 (CRUD + パスバリデーション)
│   └── frontmatter.go # frontmatterのパース・生成
├── client/
│   └── client.go      # APIクライアント (CLIから利用)
└── config/
    └── config.go      # 設定ファイル読み込み
```

## 衝突時のフロー

ファイル作成時に既存ファイルがある場合は 409 を返す。呼び出し側（Claude Code等）が以下のフローで対処する:

1. Create → 409 Conflict
2. Read で既存内容を取得
3. クライアント側でマージ
4. Update で更新

## ネットワーク

- LAN内での利用を想定
- デフォルトポート: 18484
- bind: 0.0.0.0 (LAN内アクセス用)
