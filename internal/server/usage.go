package server

import (
	"errors"
	"net/http"

	"claudio/internal/domain"
)

// registerUsageHandlers mounts the three token-usage aggregate endpoints.
func (s *Server) registerUsageHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /sessions/{id}/usage", s.getSessionUsage)
	mux.HandleFunc("GET /agents/{id}/usage", s.getAgentUsage)
	mux.HandleFunc("GET /usage", s.getGlobalUsage)
}

// getSessionUsage handles GET /sessions/{id}/usage.
// Returns 200 + UsageSummary, or 404 if the session does not exist.
func (s *Server) getSessionUsage(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	if _, err := s.store.GetSession(sid); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "session not found")
		} else {
			WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	summary, err := s.store.GetSessionUsage(sid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, summary)
}

// getAgentUsage handles GET /agents/{id}/usage.
// Returns 200 + UsageSummary, or 404 if the agent does not exist.
func (s *Server) getAgentUsage(w http.ResponseWriter, r *http.Request) {
	aid := r.PathValue("id")
	if _, err := s.store.GetAgent(aid); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "agent not found")
		} else {
			WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	summary, err := s.store.GetAgentUsage(aid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, summary)
}

// getGlobalUsage handles GET /usage.
// Returns 200 + UsageSummary aggregated across all data.
func (s *Server) getGlobalUsage(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.GetGlobalUsage()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, summary)
}
