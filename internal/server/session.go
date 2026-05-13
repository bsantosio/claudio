package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"claudio/internal/domain"
)

func (s *Server) registerSessionHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /agents/{aid}/sessions", func(w http.ResponseWriter, r *http.Request) {
		aid := r.PathValue("aid")
		if _, ok := s.store.GetAgent(aid); !ok {
			WriteError(w, http.StatusNotFound, "agent not found")
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		sess, err := s.store.CreateSession(aid, input.Name)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusCreated, sess)
	})

	mux.HandleFunc("GET /agents/{aid}/sessions", func(w http.ResponseWriter, r *http.Request) {
		aid := r.PathValue("aid")
		if _, ok := s.store.GetAgent(aid); !ok {
			WriteError(w, http.StatusNotFound, "agent not found")
			return
		}
		list, err := s.store.ListSessionsByAgent(aid)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, list)
	})

	mux.HandleFunc("GET /sessions/{sid}", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		sess, ok := s.store.GetSession(sid)
		if !ok {
			WriteError(w, http.StatusNotFound, "session not found")
			return
		}
		WriteJSON(w, http.StatusOK, sess)
	})

	mux.HandleFunc("DELETE /sessions/{sid}", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		err := s.store.DeleteSession(sid)
		if err != nil {
			if errors.Is(err, domain.ErrSessionBusy) {
				WriteError(w, http.StatusConflict, "session has an active stream; wait for it to complete")
			} else if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "session not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
