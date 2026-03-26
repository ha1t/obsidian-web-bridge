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
