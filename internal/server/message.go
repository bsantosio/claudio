package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
)

func RegisterMessageHandler(
	mux *http.ServeMux,
	cfg domain.Config,
	st *store.Store,
	runner claude.Runner,
) {
	mux.HandleFunc("POST /sessions/{sid}/message", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		sess, ok := st.GetSession(sid)
		if !ok {
			WriteError(w, http.StatusNotFound, "session not found")
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
		agent, ok := st.GetAgent(sess.AgentID)
		if !ok {
			WriteError(w, http.StatusNotFound, "agent not found for session")
			return
		}
		mu := st.Mutexes.Get(sess.ID)
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
		err := runner(ctx, cfg, agent, sess.ID, input.Content, resume, func(eventType string, data []byte) error {
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
				}
				if err := json.Unmarshal(data, &resultEv); err != nil {
					return nil
				}
				resultData, _ := json.Marshal(map[string]any{
					"text":     resultEv.Result,
					"cost_usd": resultEv.TotalCost,
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
		st.SaveSession(sess)
	})

	mux.HandleFunc("GET /sessions/{sid}/messages", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("sid")
		if _, ok := st.GetSession(sid); !ok {
			WriteError(w, http.StatusNotFound, "session not found")
			return
		}
		msgs, err := store.ReadSessionMessages(cfg.WorkDir, sid)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, msgs)
	})
}
