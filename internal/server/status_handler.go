package server

import (
	"encoding/json"
	"net/http"
)

// /status returns the last DeployResult, or 204 if no deploy has been run.
// Unauthenticated — exposes only the outcome record, never tokens.
func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	last := s.lastResult
	s.mu.RUnlock()

	if last == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(last)
}
