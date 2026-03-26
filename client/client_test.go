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
