# obsidian-web-bridge

Obsidian vaultのMarkdownファイルをHTTP API経由でCRUD操作できるGoアプリケーション。
単一バイナリにAPIサーバーとCLIクライアントを同居させており、複数端末からノートの作成・取得・更新・削除が可能。

## ビルド

```bash
go build -o obsidian-web-bridge .
```

Go 1.26 以上が必要。古い Go でも `GOTOOLCHAIN=auto`（既定）なら必要なツールチェーンが自動取得される。

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

## リモートMCP（claude.ai カスタムコネクタ）

`config.yaml` に `mcp` セクションを追加すると、API とは別に MCP エンドポイント（既定 `127.0.0.1:18485`、パス `/mcp`）が起動する。
Cloudflare Tunnel + Cloudflare Access（Managed OAuth）経由で公開し、claude.ai のカスタムコネクタからVaultのノートを読み書きできる。
`mcp` セクションが無ければ従来どおり API だけが起動する。

### 設定

```yaml
mcp:
  bind: "127.0.0.1"
  port: 18485
  cf_access_team_domain: "<team>.cloudflareaccess.com"
  cf_access_aud: "<Application Audience Tag>"
  timezone: "Asia/Tokyo"          # DailyNote の「今日」の判定に使う
  readonly_folders:               # 書き込み禁止フォルダ（Vault 直下のフォルダ名。/ を含む指定は起動エラー）
    - DailyReports
    - MorningBrief
    - Calendar
    - Garmin
    - MonthlyReview
    - Templates
    - bases
```

`cf_access_team_domain` と `cf_access_aud` が空だと起動エラーになる（認証なしで公開される事故を防ぐため）。
すべてのリクエストで Cloudflare Access の JWT を検証し、失敗は一律 403 を返す（理由はログにだけ出す）。

### サーバー側の手順

1. `config.yaml` に上記の `mcp` セクションを追加する
2. Vault の `.gitignore` に `.owb-*.tmp` を追加する（書き込みは同じフォルダの一時ファイル経由で行うため、まれに残った一時ファイルをコミットしないように）
3. ビルドして再起動する: `go build -o obsidian-web-bridge . && systemctl --user restart obsidian-web-bridge`
4. JWT なしで拒否されることを確認する: `curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18485/mcp` が `403` になる

### Cloudflare 側の手順

1. Tunnel に公開ホスト名を追加し、`http://localhost:18485` に向ける（ホスト名は任意に決める）。Tunnel の管理方法によって手順が違う
   - **ローカル管理の Tunnel**（`/etc/cloudflared/config.yml` に `ingress` を書いている場合）: `ingress` の最後の `- service: http_status:404` より前に次を足す

     ```yaml
     ingress:
       # ...既存のルール...
       - hostname: <ホスト名>
         service: http://localhost:18485
       - service: http_status:404
     ```

     DNS を登録して再起動する: `cloudflared tunnel route dns <tunnel名> <ホスト名>` → `sudo systemctl restart cloudflared`
   - **ダッシュボード管理の Tunnel**: Zero Trust → Networks → Tunnels でその Tunnel の Public Hostname を追加し、`http://localhost:18485` に向ける。`localhost` はコネクタ（cloudflared）が動くマシンを指すので、その Tunnel のコネクタがこのサーバー上で動いている場合にだけ使える
   - cloudflared を `loglevel: debug` で動かしているとリクエストヘッダー（JWT）がログに出ることがある。出ていないか確認し、普段は `info` にしておく
2. Zero Trust → Access でそのホスト名のアプリを作り、Managed OAuth を有効化する。ポリシーは自分のメールのみ許可
3. アプリの AUD タグとチームドメインを `config.yaml` の `mcp` に入れて再起動する
4. claude.ai の設定 → コネクタ → カスタムコネクタ追加で `https://<ホスト名>/mcp` を登録する

### ツール一覧

| ツール | 説明 |
|---|---|
| `list_notes` | フォルダ内のノートを更新日時の新しい順に一覧する |
| `search_notes` | ファイル名と本文を部分一致（大文字小文字を区別しない）で検索し、抜粋を返す |
| `read_note` | ノートの全文、パース済み frontmatter、`hash`、更新日時を返す |
| `create_note` | ノートを新規作成する（既存なら失敗） |
| `append_note` | ノート末尾、または指定した見出しのセクション末尾に追記する |
| `update_note` | `read_note` で得た `hash` が一致するときだけ全文を置換する |
| `append_daily_memo` | `DailyNotes/<日付>.md` の `# Memo` セクションに追記する（無ければテンプレートから作成） |
| `vault_stats` | 直下のフォルダごとのファイル数・合計サイズ（Markdown/画像/PDF/その他の内訳つき）と、`.git`・その他の隠しフォルダ・全体の合計を返す |

`readonly_folders` と `DailyNotes` への書き込みは、`append_daily_memo` 以外では拒否される。
`.git` や `.obsidian` など `.` で始まるディレクトリは読み書き・一覧・検索の対象外。
サーバーは `created` / `modified` を書かない（pre-commit フックが管理する）。

### 注意: DailyNote の同期衝突

サーバーが DailyNote を新規作成すると、Vault の自動 sync で他の端末に反映されるまで最長1時間かかる。
その間に他の端末で同じ日の DailyNote を開いて編集すると、同期時に衝突する。
サーバーが作った DailyNote が同期されるまでの最長1時間は、他の端末で同じ日の DailyNote を開かないこと。

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
