package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"claudio/internal/domain"
	"claudio/internal/store"
)

func RegisterAgentHandlers(mux *http.ServeMux, cfg domain.Config, st *store.Store) {
	mux.HandleFunc("POST /agents", func(w http.ResponseWriter, r *http.Request) {
		var input domain.Agent
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		a, err := st.CreateAgent(input, cfg.DefaultModel)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusCreated, a)
	})

	mux.HandleFunc("GET /agents", func(w http.ResponseWriter, r *http.Request) {
		all := st.ListAgents()
		if all == nil {
			all = []*domain.Agent{}
		}
		WriteJSON(w, http.StatusOK, all)
	})

	mux.HandleFunc("GET /agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, ok := st.GetAgent(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "agent not found")
			return
		}
		WriteJSON(w, http.StatusOK, a)
	})

	mux.HandleFunc("PUT /agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var input domain.Agent
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		a, err := st.UpdateAgent(id, input, cfg.DefaultModel)
		if err != nil {
			if err.Error() == "not found" {
				WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				WriteError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		WriteJSON(w, http.StatusOK, a)
	})

	mux.HandleFunc("DELETE /agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		err := st.DeleteAgent(id)
		if err != nil {
			if errors.Is(err, domain.ErrHasActiveSessions) {
				WriteError(w, http.StatusConflict, "agent has active sessions; delete sessions first")
			} else if err.Error() == "not found" {
				WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
