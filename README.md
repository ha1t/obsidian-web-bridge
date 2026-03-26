# obsidian-web-bridge

Obsidian vaultのMarkdownファイルをHTTP API経由でCRUD操作できるGoアプリケーション。
単一バイナリにAPIサーバーとCLIクライアントを同居させており、複数端末からノートの作成・取得・更新・削除が可能。

## ビルド

```bash
go build -o obsidian-web-bridge .
```

## サーバー起動

```bash
# 設定ファイルを使う場合
cp config.yaml.example config.yaml
vim config.yaml  # vault_path と api_key を設定
./obsidian-web-bridge server --config config.yaml

# フラグで直接指定する場合
./obsidian-web-bridge server --vault /path/to/ObsidianVault --server-api-key "your-secret-key"
```

### 設定項目

| 項目 | デフォルト | 説明 |
|------|-----------|------|
| `vault_path` | (必須) | Obsidian vaultのパス |
| `api_key` | (必須) | API認証キー |
| `port` | 18484 | ポート番号 |
| `bind` | 0.0.0.0 | バインドアドレス |

## CLI

環境変数で接続先を設定しておくと便利。

```bash
export OWB_SERVER=http://<server-ip>:18484
export OWB_API_KEY=your-secret-key
```

### 作成

```bash
# --body で直接指定
obsidian-web-bridge create --path "DailyReports/2026-03-26.md" \
  --title "日報" --tag daily-report --body "## 作業内容"

# 標準入力から
cat report.md | obsidian-web-bridge create --path "Notes/report.md"

# ファイルから
obsidian-web-bridge create --path "Notes/report.md" --file ./report.md
```

### 取得

```bash
obsidian-web-bridge read --path "DailyReports/2026-03-26.md"
```

### 更新

```bash
obsidian-web-bridge update --path "DailyReports/2026-03-26.md" --body "更新後の内容"
```

### 削除

```bash
obsidian-web-bridge delete --path "DailyReports/2026-03-26.md"
```

## API

ベースURL: `http://<host>:<port>/api/v1`

認証: `Authorization: Bearer <API_KEY>` ヘッダー

| メソッド | パス | 説明 |
|---------|------|------|
| POST | /notes | ノート作成 (既存なら 409) |
| GET | /notes?path=... | ノート取得 |
| PUT | /notes | ノート更新 (存在しなければ 404) |
| DELETE | /notes | ノート削除 |

### リクエスト例

```bash
# 作成
curl -X POST http://localhost:18484/api/v1/notes \
  -H "Authorization: Bearer your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"Notes/test.md","frontmatter":{"title":"テスト"},"body":"本文"}'

# 取得
curl http://localhost:18484/api/v1/notes?path=Notes/test.md \
  -H "Authorization: Bearer your-secret-key"
```

## 衝突時の対処

ファイル作成時に既存ファイルがある場合は `409 Conflict` が返る。
呼び出し側で以下のフローで対処できる:

1. Create → 409
2. Read で既存内容を取得
3. クライアント側でマージ
4. Update で更新

## テスト

```bash
go test ./...
```
