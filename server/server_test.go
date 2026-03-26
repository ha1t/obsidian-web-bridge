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

	req := httptest.NewRequest("POST", "/api/v1/notes", bytes.NewReader(jsonBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

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
