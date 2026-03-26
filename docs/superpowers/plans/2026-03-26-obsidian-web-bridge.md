# Obsidian Web Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Obsidian vaultのMarkdownファイルをHTTP API + CLIでCRUD操作できるGoアプリケーションを構築する

**Architecture:** 単一バイナリにAPIサーバー(net/http)とCLIクライアント(cobra)を同居。vault/パッケージがファイルシステム操作のコア、server/がHTTPレイヤー、client/がAPIクライアント、cmd/がCLIエントリポイント。

**Tech Stack:** Go, cobra, net/http, gopkg.in/yaml.v3

---

## File Structure

| File | Responsibility |
|------|---------------|
| `main.go` | エントリポイント。cmd.Execute()を呼ぶだけ |
| `go.mod` | モジュール定義 |
| `cmd/root.go` | cobra rootコマンド + 共通フラグ(--server, --api-key) |
| `cmd/server.go` | serverサブコマンド(--config, --vault, --port, --api-key, --bind) |
| `cmd/create.go` | createサブコマンド(--path, --title, --tag, --body, --file) |
| `cmd/read.go` | readサブコマンド(--path) |
| `cmd/update.go` | updateサブコマンド(--path, --title, --tag, --body, --file) |
| `cmd/delete.go` | deleteサブコマンド(--path) |
| `vault/vault.go` | Vault構造体、CRUD操作、パスバリデーション |
| `vault/vault_test.go` | vaultパッケージのテスト |
| `vault/frontmatter.go` | frontmatterのパース(Markdown→構造体)・生成(構造体→Markdown) |
| `vault/frontmatter_test.go` | frontmatterパッケージのテスト |
| `server/server.go` | Server構造体、ルーティング、起動 |
| `server/handler.go` | HTTPハンドラー(Create/Read/Update/Delete) |
| `server/middleware.go` | APIキー認証ミドルウェア |
| `server/server_test.go` | サーバーのテスト(httptest使用) |
| `client/client.go` | Client構造体、Create/Read/Update/Delete メソッド |
| `client/client_test.go` | クライアントのテスト(httptest使用) |
| `config/config.go` | Config構造体、YAMLファイル読み込み |
| `config/config_test.go` | configのテスト |

---

### Task 1: プロジェクト初期化

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/root.go`

- [ ] **Step 1: Go module初期化**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go mod init github.com/ha1t/obsidian-web-bridge`

- [ ] **Step 2: cobra依存を追加**

Run: `go get github.com/spf13/cobra@latest`

- [ ] **Step 3: main.goを作成**

```go
package main

import "github.com/ha1t/obsidian-web-bridge/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 4: cmd/root.goを作成**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	apiKey     string
)

var rootCmd = &cobra.Command{
	Use:   "obsidian-web-bridge",
	Short: "Obsidian vault操作用のAPIサーバー & CLI",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "", "APIサーバーのアドレス (env: OWB_SERVER)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "APIキー (env: OWB_API_KEY)")
}

func getServerAddr() string {
	if serverAddr != "" {
		return serverAddr
	}
	if v := os.Getenv("OWB_SERVER"); v != "" {
		return v
	}
	return "http://localhost:18484"
}

func getAPIKey() string {
	if apiKey != "" {
		return apiKey
	}
	return os.Getenv("OWB_API_KEY")
}
```

- [ ] **Step 5: ビルドして動作確認**

Run: `go build -o obsidian-web-bridge . && ./obsidian-web-bridge --help`
Expected: ヘルプメッセージが表示される

- [ ] **Step 6: コミット**

```bash
git add main.go go.mod go.sum cmd/root.go
git commit -m "feat: プロジェクト初期化 (cobra root command)"
```

---

### Task 2: frontmatterパッケージ

**Files:**
- Create: `vault/frontmatter.go`
- Create: `vault/frontmatter_test.go`

- [ ] **Step 1: テストを書く**

```go
package vault

import (
	"testing"
)

func TestMarshalFrontmatter(t *testing.T) {
	fm := map[string]interface{}{
		"title":   "テストノート",
		"created": "2026-03-26 18:00",
		"tags":    []interface{}{"daily-report", "test"},
	}
	body := "## 本文\n\nテスト内容です。"

	result := MarshalNote(fm, body)

	// frontmatterブロックが含まれること
	if len(result) < 3 || result[:3] != "---" {
		t.Fatalf("frontmatterブロックが見つからない: %s", result)
	}

	// bodyが含まれること
	if !containsString(result, "## 本文") {
		t.Fatalf("bodyが含まれていない: %s", result)
	}

	if !containsString(result, "テスト内容です。") {
		t.Fatalf("body内容が含まれていない: %s", result)
	}
}

func TestUnmarshalFrontmatter(t *testing.T) {
	content := `---
title: テストノート
created: "2026-03-26 18:00"
tags:
  - daily-report
  - test
---

## 本文

テスト内容です。`

	fm, body, err := UnmarshalNote(content)
	if err != nil {
		t.Fatalf("UnmarshalNote失敗: %v", err)
	}

	if fm["title"] != "テストノート" {
		t.Errorf("title = %v, want テストノート", fm["title"])
	}

	tags, ok := fm["tags"].([]interface{})
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want [daily-report, test]", fm["tags"])
	}

	if body != "## 本文\n\nテスト内容です。" {
		t.Errorf("body = %q, want %q", body, "## 本文\n\nテスト内容です。")
	}
}

func TestUnmarshalNoFrontmatter(t *testing.T) {
	content := "## ただのMarkdown\n\n本文のみ。"

	fm, body, err := UnmarshalNote(content)
	if err != nil {
		t.Fatalf("UnmarshalNote失敗: %v", err)
	}

	if len(fm) != 0 {
		t.Errorf("fm = %v, want empty map", fm)
	}

	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./vault/ -v`
Expected: FAIL (MarshalNote, UnmarshalNote が未定義)

- [ ] **Step 3: frontmatter.goを実装**

```go
package vault

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// MarshalNote はfrontmatterとbodyからMarkdown文字列を生成する。
func MarshalNote(frontmatter map[string]interface{}, body string) string {
	var sb strings.Builder

	if len(frontmatter) > 0 {
		sb.WriteString("---\n")
		yamlBytes, err := yaml.Marshal(frontmatter)
		if err == nil {
			sb.Write(yamlBytes)
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString(body)
	return sb.String()
}

// UnmarshalNote はMarkdown文字列をfrontmatterとbodyに分離する。
func UnmarshalNote(content string) (map[string]interface{}, string, error) {
	frontmatter := make(map[string]interface{})

	if !strings.HasPrefix(content, "---\n") {
		return frontmatter, content, nil
	}

	// 2つ目の---を探す
	rest := content[4:] // "---\n" の後
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return frontmatter, content, nil
	}

	yamlStr := rest[:endIdx]
	if err := yaml.Unmarshal([]byte(yamlStr), &frontmatter); err != nil {
		return nil, "", err
	}

	// ---の後の本文を取得（先頭の改行を除去）
	body := rest[endIdx+4:] // "\n---" の後
	body = strings.TrimPrefix(body, "\n")

	return frontmatter, body, nil
}
```

- [ ] **Step 4: yaml依存を追加してテスト実行**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go get gopkg.in/yaml.v3 && go test ./vault/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add vault/frontmatter.go vault/frontmatter_test.go go.mod go.sum
git commit -m "feat: frontmatterパース・生成パッケージ"
```

---

### Task 3: vaultパッケージ (CRUD + パスバリデーション)

**Files:**
- Create: `vault/vault.go`
- Create: `vault/vault_test.go`

- [ ] **Step 1: テストを書く**

```go
package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestVault(t *testing.T) (string, *Vault) {
	t.Helper()
	dir := t.TempDir()
	v := New(dir)
	return dir, v
}

func TestValidatePath(t *testing.T) {
	_, v := setupTestVault(t)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid path", "Notes/test.md", false},
		{"valid nested", "DailyReports/2026/03/report.md", false},
		{"traversal attack", "../etc/passwd", true},
		{"traversal in middle", "Notes/../../../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
		{"no md extension", "Notes/test.txt", true},
		{"empty path", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	dir, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	err := v.Create("Notes/test.md", fm, "本文です")
	if err != nil {
		t.Fatalf("Create失敗: %v", err)
	}

	// ファイルが作成されたか
	content, err := os.ReadFile(filepath.Join(dir, "Notes", "test.md"))
	if err != nil {
		t.Fatalf("ファイル読み取り失敗: %v", err)
	}
	if !stringContains(string(content), "本文です") {
		t.Errorf("ファイル内容に本文が含まれない: %s", content)
	}
}

func TestCreateConflict(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "1回目")

	err := v.Create("Notes/test.md", fm, "2回目")
	if err == nil {
		t.Fatal("2回目のCreateがエラーにならなかった")
	}
	if !IsErrAlreadyExists(err) {
		t.Errorf("ErrAlreadyExistsでない: %v", err)
	}
}

func TestRead(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "本文です")

	gotFm, gotBody, err := v.Read("Notes/test.md")
	if err != nil {
		t.Fatalf("Read失敗: %v", err)
	}
	if gotFm["title"] != "テスト" {
		t.Errorf("title = %v, want テスト", gotFm["title"])
	}
	if gotBody != "本文です" {
		t.Errorf("body = %q, want 本文です", gotBody)
	}
}

func TestReadNotFound(t *testing.T) {
	_, v := setupTestVault(t)

	_, _, err := v.Read("Notes/nonexistent.md")
	if err == nil {
		t.Fatal("存在しないファイルのReadがエラーにならなかった")
	}
	if !IsErrNotFound(err) {
		t.Errorf("ErrNotFoundでない: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "元の内容")

	newFm := map[string]interface{}{"title": "更新済み"}
	err := v.Update("Notes/test.md", newFm, "新しい内容")
	if err != nil {
		t.Fatalf("Update失敗: %v", err)
	}

	gotFm, gotBody, err := v.Read("Notes/test.md")
	if err != nil {
		t.Fatalf("Read失敗: %v", err)
	}
	if gotFm["title"] != "更新済み" {
		t.Errorf("title = %v, want 更新済み", gotFm["title"])
	}
	if gotBody != "新しい内容" {
		t.Errorf("body = %q, want 新しい内容", gotBody)
	}
}

func TestUpdateNotFound(t *testing.T) {
	_, v := setupTestVault(t)

	err := v.Update("Notes/nonexistent.md", nil, "内容")
	if err == nil {
		t.Fatal("存在しないファイルのUpdateがエラーにならなかった")
	}
	if !IsErrNotFound(err) {
		t.Errorf("ErrNotFoundでない: %v", err)
	}
}

func TestDelete(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "本文")

	err := v.Delete("Notes/test.md")
	if err != nil {
		t.Fatalf("Delete失敗: %v", err)
	}

	_, _, err = v.Read("Notes/test.md")
	if !IsErrNotFound(err) {
		t.Errorf("削除後のReadがNotFoundでない: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	_, v := setupTestVault(t)

	err := v.Delete("Notes/nonexistent.md")
	if err == nil {
		t.Fatal("存在しないファイルのDeleteがエラーにならなかった")
	}
	if !IsErrNotFound(err) {
		t.Errorf("ErrNotFoundでない: %v", err)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./vault/ -v`
Expected: FAIL (Vault, New 等が未定義)

- [ ] **Step 3: vault.goを実装**

```go
package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	errAlreadyExists = errors.New("file already exists")
	errNotFound      = errors.New("file not found")
)

func IsErrAlreadyExists(err error) bool {
	return errors.Is(err, errAlreadyExists)
}

func IsErrNotFound(err error) bool {
	return errors.Is(err, errNotFound)
}

type Vault struct {
	rootPath string
}

func New(rootPath string) *Vault {
	return &Vault{rootPath: rootPath}
}

func (v *Vault) ValidatePath(notePath string) error {
	if notePath == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(notePath) {
		return fmt.Errorf("absolute path not allowed: %s", notePath)
	}
	if strings.Contains(notePath, "..") {
		return fmt.Errorf("path traversal not allowed: %s", notePath)
	}
	if filepath.Ext(notePath) != ".md" {
		return fmt.Errorf("only .md files allowed: %s", notePath)
	}
	return nil
}

func (v *Vault) fullPath(notePath string) string {
	return filepath.Join(v.rootPath, notePath)
}

func (v *Vault) Create(notePath string, frontmatter map[string]interface{}, body string) error {
	if err := v.ValidatePath(notePath); err != nil {
		return err
	}

	fp := v.fullPath(notePath)
	if _, err := os.Stat(fp); err == nil {
		return fmt.Errorf("%w: %s", errAlreadyExists, notePath)
	}

	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	content := MarshalNote(frontmatter, body)
	return os.WriteFile(fp, []byte(content), 0644)
}

func (v *Vault) Read(notePath string) (map[string]interface{}, string, error) {
	if err := v.ValidatePath(notePath); err != nil {
		return nil, "", err
	}

	fp := v.fullPath(notePath)
	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("%w: %s", errNotFound, notePath)
		}
		return nil, "", err
	}

	return UnmarshalNote(string(data))
}

func (v *Vault) Update(notePath string, frontmatter map[string]interface{}, body string) error {
	if err := v.ValidatePath(notePath); err != nil {
		return err
	}

	fp := v.fullPath(notePath)
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", errNotFound, notePath)
	}

	content := MarshalNote(frontmatter, body)
	return os.WriteFile(fp, []byte(content), 0644)
}

func (v *Vault) Delete(notePath string) error {
	if err := v.ValidatePath(notePath); err != nil {
		return err
	}

	fp := v.fullPath(notePath)
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", errNotFound, notePath)
	}

	return os.Remove(fp)
}
```

- [ ] **Step 4: テスト実行**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./vault/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add vault/vault.go vault/vault_test.go
git commit -m "feat: vaultパッケージ (CRUD + パスバリデーション)"
```

---

### Task 4: configパッケージ

**Files:**
- Create: `config/config.go`
- Create: `config/config_test.go`

- [ ] **Step 1: テストを書く**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `vault_path: /home/ha1t/src/ObsidianGitVault
port: 18484
api_key: "test-key-123"
bind: "0.0.0.0"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}

	if cfg.VaultPath != "/home/ha1t/src/ObsidianGitVault" {
		t.Errorf("VaultPath = %q, want /home/ha1t/src/ObsidianGitVault", cfg.VaultPath)
	}
	if cfg.Port != 18484 {
		t.Errorf("Port = %d, want 18484", cfg.Port)
	}
	if cfg.APIKey != "test-key-123" {
		t.Errorf("APIKey = %q, want test-key-123", cfg.APIKey)
	}
	if cfg.Bind != "0.0.0.0" {
		t.Errorf("Bind = %q, want 0.0.0.0", cfg.Bind)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `vault_path: /tmp/vault
api_key: "key"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}

	if cfg.Port != 18484 {
		t.Errorf("Port default = %d, want 18484", cfg.Port)
	}
	if cfg.Bind != "0.0.0.0" {
		t.Errorf("Bind default = %q, want 0.0.0.0", cfg.Bind)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./config/ -v`
Expected: FAIL

- [ ] **Step 3: config.goを実装**

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	VaultPath string `yaml:"vault_path"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key"`
	Bind      string `yaml:"bind"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg := &Config{
		Port: 18484,
		Bind: "0.0.0.0",
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.VaultPath == "" {
		return nil, fmt.Errorf("vault_path is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	return cfg, nil
}
```

- [ ] **Step 4: テスト実行**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./config/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add config/config.go config/config_test.go
git commit -m "feat: configパッケージ (YAML設定読み込み)"
```

---

### Task 5: serverパッケージ (認証ミドルウェア + ハンドラー)

**Files:**
- Create: `server/server.go`
- Create: `server/middleware.go`
- Create: `server/handler.go`
- Create: `server/server_test.go`

- [ ] **Step 1: テストを書く**

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ha1t/obsidian-web-bridge/vault"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	v := vault.New(dir)
	return NewServer(v, "test-api-key")
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/notes?path=test.md", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// 404はOK（ファイルがないだけで認証は通っている）
	if w.Code == http.StatusUnauthorized {
		t.Errorf("認証が通るべきなのに401が返った")
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/notes?path=test.md", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
}

func TestAuthMiddleware_NoHeader(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/notes?path=test.md", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
}

func TestCreateNote(t *testing.T) {
	s := setupTestServer(t)
	body := NoteRequest{
		Path:        "Notes/test.md",
		Frontmatter: map[string]interface{}{"title": "テスト"},
		Body:        "本文です",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("code = %d, want 201, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateNoteConflict(t *testing.T) {
	s := setupTestServer(t)
	body := NoteRequest{
		Path:        "Notes/test.md",
		Frontmatter: map[string]interface{}{"title": "テスト"},
		Body:        "本文です",
	}
	jsonBody, _ := json.Marshal(body)

	// 1回目
	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// 2回目
	req = httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("code = %d, want 409", w.Code)
	}
}

func TestReadNote(t *testing.T) {
	s := setupTestServer(t)

	// まず作成
	body := NoteRequest{
		Path:        "Notes/test.md",
		Frontmatter: map[string]interface{}{"title": "テスト"},
		Body:        "本文です",
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// Read
	req = httptest.NewRequest("GET", "/api/v1/notes?path=Notes/test.md", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("code = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var resp NoteResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Body != "本文です" {
		t.Errorf("body = %q, want 本文です", resp.Body)
	}
}

func TestReadNoteNotFound(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/notes?path=Notes/nonexistent.md", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", w.Code)
	}
}

func TestUpdateNote(t *testing.T) {
	s := setupTestServer(t)

	// まず作成
	body := NoteRequest{
		Path:        "Notes/test.md",
		Frontmatter: map[string]interface{}{"title": "テスト"},
		Body:        "元の内容",
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// Update
	body.Body = "新しい内容"
	body.Frontmatter["title"] = "更新済み"
	jsonBody, _ = json.Marshal(body)
	req = httptest.NewRequest("PUT", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("code = %d, want 200, body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteNote(t *testing.T) {
	s := setupTestServer(t)

	// まず作成
	body := NoteRequest{
		Path:        "Notes/test.md",
		Frontmatter: map[string]interface{}{"title": "テスト"},
		Body:        "本文",
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// Delete
	delBody := NoteRequest{Path: "Notes/test.md"}
	jsonBody, _ = json.Marshal(delBody)
	req = httptest.NewRequest("DELETE", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", w.Code)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./server/ -v`
Expected: FAIL

- [ ] **Step 3: server.goを実装**

```go
package server

import (
	"fmt"
	"net/http"

	"github.com/ha1t/obsidian-web-bridge/vault"
)

type Server struct {
	vault  *vault.Vault
	apiKey string
}

func NewServer(v *vault.Vault, apiKey string) *Server {
	return &Server{vault: v, apiKey: apiKey}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notes", s.handleNotes)
	return s.authMiddleware(mux)
}

func (s *Server) ListenAndServe(bind string, port int) error {
	addr := fmt.Sprintf("%s:%d", bind, port)
	fmt.Printf("Starting server on %s\n", addr)
	return http.ListenAndServe(addr, s.Handler())
}
```

- [ ] **Step 4: middleware.goを実装**

```go
package server

import (
	"net/http"
	"strings"
)

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, `{"error":"authorization header required"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token != s.apiKey {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 5: handler.goを実装**

```go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/ha1t/obsidian-web-bridge/vault"
)

type NoteRequest struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
}

type NoteResponse struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Created     bool                   `json:"created,omitempty"`
	Updated     bool                   `json:"updated,omitempty"`
	Deleted     bool                   `json:"deleted,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		s.handleCreate(w, r)
	case http.MethodGet:
		s.handleRead(w, r)
	case http.MethodPut:
		s.handleUpdate(w, r)
	case http.MethodDelete:
		s.handleDelete(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(NoteResponse{Error: "method not allowed"})
	}
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req NoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: "invalid request body"})
		return
	}

	err := s.vault.Create(req.Path, req.Frontmatter, req.Body)
	if err != nil {
		if vault.IsErrAlreadyExists(err) {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(NoteResponse{Error: "file already exists", Path: req.Path})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(NoteResponse{Path: req.Path, Created: true})
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	notePath := r.URL.Query().Get("path")
	if notePath == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: "path query parameter required"})
		return
	}

	fm, body, err := s.vault.Read(notePath)
	if err != nil {
		if vault.IsErrNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(NoteResponse{Error: "file not found", Path: notePath})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(NoteResponse{Path: notePath, Frontmatter: fm, Body: body})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req NoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: "invalid request body"})
		return
	}

	err := s.vault.Update(req.Path, req.Frontmatter, req.Body)
	if err != nil {
		if vault.IsErrNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(NoteResponse{Error: "file not found", Path: req.Path})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(NoteResponse{Path: req.Path, Updated: true})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req NoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: "invalid request body"})
		return
	}

	err := s.vault.Delete(req.Path)
	if err != nil {
		if vault.IsErrNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(NoteResponse{Error: "file not found", Path: req.Path})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(NoteResponse{Error: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(NoteResponse{Path: req.Path, Deleted: true})
}
```

- [ ] **Step 6: テスト実行**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./server/ -v`
Expected: PASS

- [ ] **Step 7: コミット**

```bash
git add server/server.go server/middleware.go server/handler.go server/server_test.go
git commit -m "feat: serverパッケージ (認証ミドルウェア + CRUDハンドラー)"
```

---

### Task 6: clientパッケージ

**Files:**
- Create: `client/client.go`
- Create: `client/client_test.go`

- [ ] **Step 1: テストを書く**

```go
package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockNoteResponse struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Created     bool                   `json:"created,omitempty"`
	Updated     bool                   `json:"updated,omitempty"`
	Deleted     bool                   `json:"deleted,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func TestClientCreate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header missing or wrong")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(mockNoteResponse{Path: "Notes/test.md", Created: true})
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key")
	err := c.Create("Notes/test.md", map[string]interface{}{"title": "test"}, "body")
	if err != nil {
		t.Fatalf("Create失敗: %v", err)
	}
}

func TestClientCreateConflict(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(mockNoteResponse{Error: "file already exists", Path: "Notes/test.md"})
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key")
	err := c.Create("Notes/test.md", nil, "body")
	if err == nil {
		t.Fatal("409でエラーが返るべき")
	}
}

func TestClientRead(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Query().Get("path") != "Notes/test.md" {
			t.Errorf("path = %q, want Notes/test.md", r.URL.Query().Get("path"))
		}
		json.NewEncoder(w).Encode(mockNoteResponse{
			Path:        "Notes/test.md",
			Frontmatter: map[string]interface{}{"title": "test"},
			Body:        "本文",
		})
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key")
	fm, body, err := c.Read("Notes/test.md")
	if err != nil {
		t.Fatalf("Read失敗: %v", err)
	}
	if fm["title"] != "test" {
		t.Errorf("title = %v, want test", fm["title"])
	}
	if body != "本文" {
		t.Errorf("body = %q, want 本文", body)
	}
}

func TestClientUpdate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		json.NewEncoder(w).Encode(mockNoteResponse{Path: "Notes/test.md", Updated: true})
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key")
	err := c.Update("Notes/test.md", nil, "new body")
	if err != nil {
		t.Fatalf("Update失敗: %v", err)
	}
}

func TestClientDelete(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		json.NewEncoder(w).Encode(mockNoteResponse{Path: "Notes/test.md", Deleted: true})
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key")
	err := c.Delete("Notes/test.md")
	if err != nil {
		t.Fatalf("Delete失敗: %v", err)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./client/ -v`
Expected: FAIL

- [ ] **Step 3: client.goを実装**

```go
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type noteRequest struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
}

type noteResponse struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Created     bool                   `json:"created,omitempty"`
	Updated     bool                   `json:"updated,omitempty"`
	Deleted     bool                   `json:"deleted,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

func (c *Client) doJSON(method, path string, reqBody interface{}) (*noteResponse, error) {
	var bodyBuf bytes.Buffer
	if reqBody != nil {
		if err := json.NewEncoder(&bodyBuf).Encode(reqBody); err != nil {
			return nil, fmt.Errorf("failed to encode request: %w", err)
		}
	}

	req, err := http.NewRequest(method, c.baseURL+path, &bodyBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var noteResp noteResponse
	if err := json.NewDecoder(resp.Body).Decode(&noteResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &noteResp, fmt.Errorf("API error (%d): %s", resp.StatusCode, noteResp.Error)
	}

	return &noteResp, nil
}

func (c *Client) Create(notePath string, frontmatter map[string]interface{}, body string) error {
	_, err := c.doJSON("POST", "/api/v1/notes", noteRequest{
		Path:        notePath,
		Frontmatter: frontmatter,
		Body:        body,
	})
	return err
}

func (c *Client) Read(notePath string) (map[string]interface{}, string, error) {
	resp, err := c.doJSON("GET", "/api/v1/notes?path="+url.QueryEscape(notePath), nil)
	if err != nil {
		return nil, "", err
	}
	return resp.Frontmatter, resp.Body, nil
}

func (c *Client) Update(notePath string, frontmatter map[string]interface{}, body string) error {
	_, err := c.doJSON("PUT", "/api/v1/notes", noteRequest{
		Path:        notePath,
		Frontmatter: frontmatter,
		Body:        body,
	})
	return err
}

func (c *Client) Delete(notePath string) error {
	_, err := c.doJSON("DELETE", "/api/v1/notes", noteRequest{
		Path: notePath,
	})
	return err
}
```

- [ ] **Step 4: テスト実行**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test ./client/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add client/client.go client/client_test.go
git commit -m "feat: clientパッケージ (APIクライアント)"
```

---

### Task 7: CLIコマンド (server + CRUD)

**Files:**
- Create: `cmd/server.go`
- Create: `cmd/create.go`
- Create: `cmd/read.go`
- Create: `cmd/update.go`
- Create: `cmd/delete.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: cmd/server.goを作成**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/ha1t/obsidian-web-bridge/config"
	"github.com/ha1t/obsidian-web-bridge/server"
	"github.com/ha1t/obsidian-web-bridge/vault"
	"github.com/spf13/cobra"
)

var (
	configPath  string
	vaultPath   string
	port        int
	serverKey   string
	bindAddress string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "APIサーバーを起動",
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *config.Config
		var err error

		if configPath != "" {
			cfg, err = config.Load(configPath)
			if err != nil {
				return fmt.Errorf("設定ファイル読み込み失敗: %w", err)
			}
		} else {
			cfg = &config.Config{
				VaultPath: vaultPath,
				Port:      port,
				APIKey:    serverKey,
				Bind:      bindAddress,
			}
		}

		// フラグで上書き
		if cmd.Flags().Changed("vault") {
			cfg.VaultPath = vaultPath
		}
		if cmd.Flags().Changed("port") {
			cfg.Port = port
		}
		if cmd.Flags().Changed("api-key") && serverKey != "" {
			cfg.APIKey = serverKey
		}
		if cmd.Flags().Changed("bind") {
			cfg.Bind = bindAddress
		}

		if cfg.VaultPath == "" {
			return fmt.Errorf("vault pathを指定してください (--vault または config)")
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("API keyを指定してください (--api-key または config)")
		}
		if cfg.Port == 0 {
			cfg.Port = 18484
		}
		if cfg.Bind == "" {
			cfg.Bind = "0.0.0.0"
		}

		// vault存在確認
		if _, err := os.Stat(cfg.VaultPath); os.IsNotExist(err) {
			return fmt.Errorf("vault pathが存在しません: %s", cfg.VaultPath)
		}

		v := vault.New(cfg.VaultPath)
		s := server.NewServer(v, cfg.APIKey)
		return s.ListenAndServe(cfg.Bind, cfg.Port)
	},
}

func init() {
	serverCmd.Flags().StringVar(&configPath, "config", "", "設定ファイルパス")
	serverCmd.Flags().StringVar(&vaultPath, "vault", "", "Obsidian vaultのパス")
	serverCmd.Flags().IntVar(&port, "port", 18484, "ポート番号")
	serverCmd.Flags().StringVar(&serverKey, "server-api-key", "", "APIキー")
	serverCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0", "バインドアドレス")
	rootCmd.AddCommand(serverCmd)
}
```

- [ ] **Step 2: cmd/create.goを作成**

```go
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
)

var (
	createPath  string
	createTitle string
	createTags  []string
	createBody  string
	createFile  string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "ノートを作成",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := resolveBody(createFile, createBody)
		if err != nil {
			return err
		}

		fm := buildFrontmatter(createTitle, createTags)

		c := client.New(getServerAddr(), getAPIKey())
		if err := c.Create(createPath, fm, body); err != nil {
			return err
		}
		fmt.Printf("Created: %s\n", createPath)
		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&createPath, "path", "", "ノートのパス (必須)")
	createCmd.MarkFlagRequired("path")
	createCmd.Flags().StringVar(&createTitle, "title", "", "タイトル (frontmatter)")
	createCmd.Flags().StringSliceVar(&createTags, "tag", nil, "タグ (複数指定可)")
	createCmd.Flags().StringVar(&createBody, "body", "", "本文")
	createCmd.Flags().StringVar(&createFile, "file", "", "本文をファイルから読み込み")
	rootCmd.AddCommand(createCmd)
}

func resolveBody(filePath, bodyFlag string) (string, error) {
	// 優先順位: --file > stdin > --body
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("ファイル読み込み失敗: %w", err)
		}
		return string(data), nil
	}

	// stdinにデータがあるか確認
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("stdin読み込み失敗: %w", err)
		}
		return string(data), nil
	}

	if bodyFlag != "" {
		return bodyFlag, nil
	}

	return "", fmt.Errorf("本文を指定してください (--body, --file, またはstdin)")
}

func buildFrontmatter(title string, tags []string) map[string]interface{} {
	fm := make(map[string]interface{})
	if title != "" {
		fm["title"] = title
	}
	if len(tags) > 0 {
		tagList := make([]interface{}, len(tags))
		for i, t := range tags {
			tagList[i] = t
		}
		fm["tags"] = tagList
	}
	if len(fm) == 0 {
		return nil
	}
	return fm
}
```

- [ ] **Step 3: cmd/read.goを作成**

```go
package cmd

import (
	"fmt"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var readPath string

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "ノートを取得",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(getServerAddr(), getAPIKey())
		fm, body, err := c.Read(readPath)
		if err != nil {
			return err
		}

		// frontmatterがあれば表示
		if len(fm) > 0 {
			fmt.Println("---")
			yamlBytes, _ := yaml.Marshal(fm)
			fmt.Print(string(yamlBytes))
			fmt.Println("---")
			fmt.Println()
		}

		fmt.Print(body)
		return nil
	},
}

func init() {
	readCmd.Flags().StringVar(&readPath, "path", "", "ノートのパス (必須)")
	readCmd.MarkFlagRequired("path")
	rootCmd.AddCommand(readCmd)
}
```

- [ ] **Step 4: cmd/update.goを作成**

```go
package cmd

import (
	"fmt"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
)

var (
	updatePath  string
	updateTitle string
	updateTags  []string
	updateBody  string
	updateFile  string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "ノートを更新",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := resolveBody(updateFile, updateBody)
		if err != nil {
			return err
		}

		fm := buildFrontmatter(updateTitle, updateTags)

		c := client.New(getServerAddr(), getAPIKey())
		if err := c.Update(updatePath, fm, body); err != nil {
			return err
		}
		fmt.Printf("Updated: %s\n", updatePath)
		return nil
	},
}

func init() {
	updateCmd.Flags().StringVar(&updatePath, "path", "", "ノートのパス (必須)")
	updateCmd.MarkFlagRequired("path")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "タイトル (frontmatter)")
	updateCmd.Flags().StringSliceVar(&updateTags, "tag", nil, "タグ (複数指定可)")
	updateCmd.Flags().StringVar(&updateBody, "body", "", "本文")
	updateCmd.Flags().StringVar(&updateFile, "file", "", "本文をファイルから読み込み")
	rootCmd.AddCommand(updateCmd)
}
```

- [ ] **Step 5: cmd/delete.goを作成**

```go
package cmd

import (
	"fmt"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
)

var deletePath string

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "ノートを削除",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(getServerAddr(), getAPIKey())
		if err := c.Delete(deletePath); err != nil {
			return err
		}
		fmt.Printf("Deleted: %s\n", deletePath)
		return nil
	},
}

func init() {
	deleteCmd.Flags().StringVar(&deletePath, "path", "", "ノートのパス (必須)")
	deleteCmd.MarkFlagRequired("path")
	rootCmd.AddCommand(deleteCmd)
}
```

- [ ] **Step 6: ビルドして動作確認**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go build -o obsidian-web-bridge . && ./obsidian-web-bridge --help && ./obsidian-web-bridge server --help && ./obsidian-web-bridge create --help`
Expected: 各コマンドのヘルプが表示される

- [ ] **Step 7: コミット**

```bash
git add cmd/server.go cmd/create.go cmd/read.go cmd/update.go cmd/delete.go
git commit -m "feat: CLIコマンド (server + create/read/update/delete)"
```

---

### Task 8: 結合テスト

**Files:**
- Create: `integration_test.go`

- [ ] **Step 1: 結合テストを書く**

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ha1t/obsidian-web-bridge/server"
	"github.com/ha1t/obsidian-web-bridge/vault"
)

type noteRequest struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
}

type noteResponse struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Created     bool                   `json:"created,omitempty"`
	Updated     bool                   `json:"updated,omitempty"`
	Deleted     bool                   `json:"deleted,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func TestIntegration_CRUDFlow(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	s := server.NewServer(v, "test-key")
	handler := s.Handler()

	authHeader := "Bearer test-key"

	// 1. Create
	createBody := noteRequest{
		Path:        "DailyReports/2026-03-26-server1.md",
		Frontmatter: map[string]interface{}{"title": "日報 server1", "tags": []interface{}{"daily-report"}},
		Body:        "## 作業内容\n- API実装",
	}
	jsonBody, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Create: code = %d, want 201, body = %s", w.Code, w.Body.String())
	}

	// 2. Create同じパス → 409
	req = httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("Create conflict: code = %d, want 409", w.Code)
	}

	// 3. Read
	req = httptest.NewRequest("GET", "/api/v1/notes?path=DailyReports/2026-03-26-server1.md", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Read: code = %d, want 200", w.Code)
	}

	var readResp noteResponse
	json.NewDecoder(w.Body).Decode(&readResp)
	if readResp.Body != "## 作業内容\n- API実装" {
		t.Errorf("Read body = %q, want '## 作業内容\\n- API実装'", readResp.Body)
	}

	// 4. Update
	updateBody := noteRequest{
		Path:        "DailyReports/2026-03-26-server1.md",
		Frontmatter: map[string]interface{}{"title": "日報 server1 (更新)"},
		Body:        "## 作業内容\n- API実装\n- テスト追加",
	}
	jsonBody, _ = json.Marshal(updateBody)
	req = httptest.NewRequest("PUT", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Update: code = %d, want 200", w.Code)
	}

	// 5. Read again to verify update
	req = httptest.NewRequest("GET", "/api/v1/notes?path=DailyReports/2026-03-26-server1.md", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var readResp2 noteResponse
	json.NewDecoder(w.Body).Decode(&readResp2)
	if readResp2.Frontmatter["title"] != "日報 server1 (更新)" {
		t.Errorf("Updated title = %v, want '日報 server1 (更新)'", readResp2.Frontmatter["title"])
	}

	// 6. Delete
	delBody := noteRequest{Path: "DailyReports/2026-03-26-server1.md"}
	jsonBody, _ = json.Marshal(delBody)
	req = httptest.NewRequest("DELETE", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Delete: code = %d, want 200", w.Code)
	}

	// 7. Read after delete → 404
	req = httptest.NewRequest("GET", "/api/v1/notes?path=DailyReports/2026-03-26-server1.md", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Read after delete: code = %d, want 404", w.Code)
	}
}

func TestIntegration_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	s := server.NewServer(v, "test-key")
	handler := s.Handler()

	body := noteRequest{
		Path: "../../../etc/passwd",
		Body: "malicious",
	}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Path traversal: code = %d, want 400", w.Code)
	}
}

func TestIntegration_AuthRequired(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	s := server.NewServer(v, "test-key")
	handler := s.Handler()

	req := httptest.NewRequest("GET", "/api/v1/notes?path=test.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("No auth: code = %d, want 401", w.Code)
	}
}
```

- [ ] **Step 2: テスト実行**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go test -v ./...`
Expected: 全テストPASS

- [ ] **Step 3: コミット**

```bash
git add integration_test.go
git commit -m "test: 結合テスト (CRUDフロー, パストラバーサル, 認証)"
```

---

### Task 9: サンプル設定ファイルとビルド確認

**Files:**
- Create: `config.yaml.example`
- Create: `.gitignore`

- [ ] **Step 1: サンプル設定ファイルを作成**

```yaml
# Obsidian Web Bridge 設定ファイル
vault_path: /path/to/your/ObsidianVault
port: 18484
api_key: "change-me-to-a-secure-key"
bind: "0.0.0.0"
```

- [ ] **Step 2: .gitignoreを作成**

```
obsidian-web-bridge
config.yaml
*.exe
```

- [ ] **Step 3: 最終ビルド & テスト**

Run: `cd /home/ha1t/src/obsidian-web-bridge && go build -o obsidian-web-bridge . && go test ./... -v`
Expected: ビルド成功、全テストPASS

- [ ] **Step 4: コミット**

```bash
git add config.yaml.example .gitignore
git commit -m "chore: サンプル設定ファイルと.gitignore追加"
```
