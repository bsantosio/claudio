package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"claudio/internal/domain"
	"claudio/internal/store"
)

func RegisterSessionHandlers(mux *http.ServeMux, _ domain.Config, st *store.Store) {
	mux.HandleFunc("POST /agents/{aid}/sessions", func(w http.ResponseWriter, r *http.Request) {
		aid := r.PathValue("aid")
		if _, ok := st.GetAgent(aid); !ok {
			WriteError(w, http.StatusNotFound, "agent not found")
			return
		}
		var input struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		sess, err := st.CreateSession(aid, input.Name)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusCreated, sess)
	})

	mux.HandleFunc("GET /agents/{aid}/sessions", func(w http.ResponseWriter, r *http.Request) {
		aid := r.PathValue("aid")
		if _, ok := st.GetAgent(aid); !ok {
			WriteError(w, http.StatusNotFound, "agent not found")
			return
		}
		list := st.ListSessionsByAgent(aid)
		if list == nil {
			list = []*domain.Session{}
		}
		WriteJSON(w, http.StatusOK, list)
	})

	mux.HandleFunc("GET /sessions/{sid}", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		sess, ok := st.GetSession(sid)
		if !ok {
			WriteError(w, http.StatusNotFound, "session not found")
			return
		}
		WriteJSON(w, http.StatusOK, sess)
	})

	mux.HandleFunc("DELETE /sessions/{sid}", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		err := st.DeleteSession(sid)
		if err != nil {
			if errors.Is(err, domain.ErrSessionBusy) {
				WriteError(w, http.StatusConflict, "session has an active stream; wait for it to complete")
			} else if err.Error() == "not found" {
				WriteError(w, http.StatusNotFound, "session not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
