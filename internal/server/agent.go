package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"claudio/internal/claude"
	"claudio/internal/domain"
)

func (s *Server) registerAgentHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /agents", func(w http.ResponseWriter, r *http.Request) {
		var input domain.Agent
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		a, err := s.store.CreateAgent(input, s.cfg.DefaultModel)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteJSON(w, http.StatusCreated, a)
	})

	mux.HandleFunc("GET /agents", func(w http.ResponseWriter, r *http.Request) {
		all, err := s.store.ListAgents()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, all)
	})

	mux.HandleFunc("GET /agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := s.store.GetAgent(id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
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
		a, err := s.store.UpdateAgent(id, input, s.cfg.DefaultModel)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
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
		err := s.store.DeleteAgent(id)
		if err != nil {
			if errors.Is(err, domain.ErrHasActiveSessions) {
				WriteError(w, http.StatusConflict, "agent has active sessions; delete sessions first")
			} else if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /agents/generate", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if strings.TrimSpace(input.Description) == "" {
			WriteError(w, http.StatusBadRequest, "description is required")
			return
		}

		generatorAgent := &domain.Agent{
			Model:        "sonnet",
			SystemPrompt: domain.AgentGeneratorPrompt,
		}
		sessionID, _ := domain.NewUUID()

		var result strings.Builder
		ctx := r.Context()
		err := claude.RunClaude(ctx, s.cfg, generatorAgent, sessionID, input.Description, false, func(eventType string, data []byte) error {
			if eventType == "result" {
				var ev struct {
					Result string `json:"result"`
				}
				if json.Unmarshal(data, &ev) == nil {
					result.WriteString(ev.Result)
				}
			}
			return nil
		})
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "generation failed: "+err.Error())
			return
		}

		text := domain.StripCodeFences(result.String())

		var generated domain.Agent
		if err := json.Unmarshal([]byte(text), &generated); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to parse generated config — raw: "+text)
			return
		}

		WriteJSON(w, http.StatusOK, generated)
	})

	// Task 2.3 — Install agent: copies definition to .claude/agents/.
	mux.HandleFunc("POST /agents/{id}/install", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := s.store.GetAgent(id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if err := domain.InstallAgent(a, s.cfg.WorkDir); err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "installed"})
	})

	// Task 2.4 — Uninstall agent: removes definition from .claude/agents/.
	mux.HandleFunc("DELETE /agents/{id}/install", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := s.store.GetAgent(id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if err := domain.UninstallAgent(a, s.cfg.WorkDir); err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
