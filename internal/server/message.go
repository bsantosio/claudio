package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
)

func (s *Server) registerMessageHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /sessions/{sid}/message", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		sess, err := s.store.GetSession(sid)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "session not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		var input struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if strings.TrimSpace(input.Content) == "" {
			WriteError(w, http.StatusBadRequest, "content is required")
			return
		}
		agent, err := s.store.GetAgent(sess.AgentID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "agent not found for session")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		mu := s.store.Mutexes.Get(sess.ID)
		mu.Lock()
		defer mu.Unlock()
		resume := sess.TurnCount > 0
		sess.TurnCount++
		sess.LastActive = time.Now().UTC().Format(time.RFC3339Nano)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, canFlush := w.(http.Flusher)
		ctx := r.Context()
		err = s.runner(ctx, s.cfg, agent, sess.ID, input.Content, resume, func(eventType string, data []byte) error {
			var sseEvent string
			switch eventType {
			case "system":
				ev, err := claude.ParseNDJSON(string(data))
				if err != nil {
					return nil
				}
				initData, _ := json.Marshal(map[string]string{"session_id": ev.SessionID})
				sseEvent = claude.FormatSSE("session_init", string(initData))
			case "assistant":
				ev, err := claude.ParseNDJSON(string(data))
				if err != nil {
					return nil
				}
				text, err := claude.ExtractText(ev)
				if err != nil || text == "" {
					return nil
				}
				textData, _ := json.Marshal(map[string]string{"text": text})
				sseEvent = claude.FormatSSE("text_delta", string(textData))
			case "result":
				var resultEv struct {
					Result    string  `json:"result"`
					TotalCost float64 `json:"total_cost_usd"`
					SessionID string  `json:"session_id"`
					Usage     struct {
						InputTokens              int `json:"input_tokens"`
						OutputTokens             int `json:"output_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					} `json:"usage"`
					ModelUsage json.RawMessage `json:"modelUsage"`
				}
				if err := json.Unmarshal(data, &resultEv); err != nil {
					return nil
				}
				// Persist token usage — non-fatal on error (P1, P6, P7).
				tu := domain.TokenUsage{
					SessionID:                sid,
					AgentID:                  agent.ID,
					InputTokens:              resultEv.Usage.InputTokens,
					OutputTokens:             resultEv.Usage.OutputTokens,
					CacheCreationInputTokens: resultEv.Usage.CacheCreationInputTokens,
					CacheReadInputTokens:     resultEv.Usage.CacheReadInputTokens,
					TotalCostUSD:             resultEv.TotalCost,
				}
				if saveErr := s.store.SaveTokenUsage(tu); saveErr != nil {
					log.Printf("failed to save token usage: %v", saveErr)
				}
				usageData := map[string]any{
					"input_tokens":  resultEv.Usage.InputTokens,
					"output_tokens": resultEv.Usage.OutputTokens,
					"cache_creation_input_tokens": resultEv.Usage.CacheCreationInputTokens,
					"cache_read_input_tokens":     resultEv.Usage.CacheReadInputTokens,
				}
				var contextWindow, maxOutput int
				if resultEv.ModelUsage != nil {
					var mu map[string]struct {
						ContextWindow   int `json:"contextWindow"`
						MaxOutputTokens int `json:"maxOutputTokens"`
					}
					if json.Unmarshal(resultEv.ModelUsage, &mu) == nil {
						for _, v := range mu {
							contextWindow = v.ContextWindow
							maxOutput = v.MaxOutputTokens
							break
						}
					}
				}
				if contextWindow > 0 {
					usageData["context_window"] = contextWindow
				}
				if maxOutput > 0 {
					usageData["max_output_tokens"] = maxOutput
				}
				resultData, _ := json.Marshal(map[string]any{
					"text":     resultEv.Result,
					"cost_usd": resultEv.TotalCost,
					"usage":    usageData,
				})
				sseEvent = claude.FormatSSE("result", string(resultData))
			default:
				ev, err := claude.ParseNDJSON(string(data))
				if err != nil {
					return nil
				}
				sseEvent = claude.FormatSSE("raw", ev.RawLine)
			}
			if _, err := fmt.Fprint(w, sseEvent); err != nil {
				return fmt.Errorf("write SSE: %w", err)
			}
			if canFlush {
				flusher.Flush()
			}
			return nil
		})
		if err != nil {
			errData, _ := json.Marshal(map[string]string{"error": err.Error()})
			fmt.Fprint(w, claude.FormatSSE("error", string(errData)))
			if canFlush {
				flusher.Flush()
			}
		}
		if err := s.store.SaveSession(sess); err != nil {
			log.Printf("failed to save session: %v", err)
		}
	})

	mux.HandleFunc("GET /sessions/{sid}/messages", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		if _, err := s.store.GetSession(sid); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "session not found")
			} else {
				WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		msgs, err := store.ReadSessionMessages(s.cfg.WorkDir, sid)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, msgs)
	})
}
