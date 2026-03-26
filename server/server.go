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
