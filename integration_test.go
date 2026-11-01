package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ha1t/obsidian-web-bridge/cmd"
	"github.com/ha1t/obsidian-web-bridge/config"
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

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitListening(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port %d did not start listening", port)
}

func isListening(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func startServers(t *testing.T, cfg *config.Config) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cmd.RunServers(ctx, cfg) }()
	return func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("RunServers returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("RunServers did not stop after cancel")
		}
	}
}

func TestIntegration_MCPListenerRequiresJWT(t *testing.T) {
	apiPort, mcpPort := freePort(t), freePort(t)
	cfg := &config.Config{
		VaultPath: t.TempDir(),
		Port:      apiPort,
		APIKey:    "test-key",
		Bind:      "127.0.0.1",
		MCP: &config.MCPConfig{
			Bind:               "127.0.0.1",
			Port:               mcpPort,
			CFAccessTeamDomain: "example.cloudflareaccess.com",
			CFAccessAUD:        "test-aud",
			Timezone:           "Asia/Tokyo",
			ReadonlyFolders:    config.DefaultReadonlyFolders,
		},
	}
	stop := startServers(t, cfg)
	defer stop()
	waitListening(t, apiPort)
	waitListening(t, mcpPort)

	// 既存API: Bearerキー付きGETが従来どおり動く（存在しないノートは404）
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/v1/notes?path=Notes/none.md", apiPort), nil)
	req.Header.Set("Authorization", "Bearer test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("API with key: code = %d, want 404", resp.StatusCode)
	}
	req.Header.Del("Authorization")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("API without key: code = %d, want 401", resp.StatusCode)
	}

	// MCP: JWT無しのPOST /mcp は403
	resp, err = http.Post(fmt.Sprintf("http://127.0.0.1:%d/mcp", mcpPort), "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("MCP without JWT: code = %d, want 403", resp.StatusCode)
	}
}

func TestIntegration_MCPDisabledWhenNotConfigured(t *testing.T) {
	apiPort, mcpPort := freePort(t), freePort(t)
	cfg := &config.Config{
		VaultPath: t.TempDir(),
		Port:      apiPort,
		APIKey:    "test-key",
		Bind:      "127.0.0.1",
	}
	stop := startServers(t, cfg)
	defer stop()
	waitListening(t, apiPort)

	if isListening(mcpPort) {
		t.Errorf("MCP port %d is listening although cfg.MCP == nil", mcpPort)
	}
}
