package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"mphtlc-lgp-mpc/internal/orchestrator"
)

type Server struct{ mgr *orchestrator.Manager }

func NewServer(mgr *orchestrator.Manager) *Server { return &Server{mgr: mgr} }

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/keygen/start", s.handleKeygenStart)
	mux.HandleFunc("/api/v1/sign/start", s.handleSignStart)
	mux.HandleFunc("/api/v1/sessions/init", s.handleSessionInit)
	mux.HandleFunc("/api/v1/tss/message", s.handleTSSMessage)
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionGet)
	return loggingMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mgr.Health())
}
func (s *Server) handleKeygenStart(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.KeygenStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.mgr.StartKeygen(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}
func (s *Server) handleSignStart(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.SignStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.mgr.StartSign(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}
func (s *Server) handleSessionInit(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.SessionInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.mgr.InitLocalSession(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}
func (s *Server) handleTSSMessage(w http.ResponseWriter, r *http.Request) {
	var req orchestrator.TSSMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.mgr.ReceiveMessage(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}
func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	resp, err := s.mgr.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[http] %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
