package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
)

type OpenAIChatRequest struct {
	Model    string         `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   *bool          `json:"stream,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
}

type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type ChunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

var modelAliasMap = map[string]string{
	"gpt-4":         "claude-opus-4-5",
	"gpt-4-turbo":   "claude-opus-4-5",
	"gpt-4o":        "claude-sonnet-4-6",
	"gpt-3.5-turbo": "claude-haiku-4-5",
	"gpt-3.5":       "claude-haiku-4-5",
}

func NormalizeModel(m string) string {
	if mapped, ok := modelAliasMap[m]; ok {
		return mapped
	}
	return m
}

var knownClaudeModels = []ModelObject{
	{ID: "claude-haiku-4-5", Object: "model", Created: 1714000000, OwnedBy: "anthropic"},
	{ID: "claude-sonnet-4-6", Object: "model", Created: 1714000001, OwnedBy: "anthropic"},
	{ID: "claude-opus-4-5", Object: "model", Created: 1714000002, OwnedBy: "anthropic"},
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"object": "list",
		"data":   knownClaudeModels,
	}
	WriteJSON(w, http.StatusOK, resp)
}

func openaiCompletionsHandler(
	cfg domain.Config,
	st *store.Store,
	runner claude.Runner,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req OpenAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if len(req.Messages) == 0 {
			WriteError(w, http.StatusBadRequest, "messages is required")
			return
		}
		var agent *domain.Agent
		if agentID := r.Header.Get("X-Agent-Id"); agentID != "" {
			a, ok := st.GetAgent(agentID)
			if !ok {
				WriteError(w, http.StatusNotFound, "agent not found")
				return
			}
			agent = a
		} else {
			model := NormalizeModel(req.Model)
			if model == "" {
				model = cfg.DefaultModel
			}
			if !domain.KnownModels[model] {
				model = cfg.DefaultModel
			}
			systemPrompt := ""
			for _, msg := range req.Messages {
				if msg.Role == "system" {
					systemPrompt = msg.Content
					break
				}
			}
			if systemPrompt == "" {
				systemPrompt = "You are a helpful assistant."
			}
			agent = &domain.Agent{
				Model:        model,
				SystemPrompt: systemPrompt,
			}
		}
		streaming := req.Stream != nil && *req.Stream
		var promptParts []string
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				continue
			}
			promptParts = append(promptParts, msg.Content)
		}
		prompt := strings.Join(promptParts, "\n")
		if strings.TrimSpace(prompt) == "" {
			WriteError(w, http.StatusBadRequest, "no user message found")
			return
		}
		sessionID, err := domain.NewUUID()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to generate session id")
			return
		}
		resume := false
		if sid := r.Header.Get("X-Session-Id"); sid != "" {
			if sess, ok := st.GetSession(sid); ok {
				sessionID = sess.ID
				resume = sess.TurnCount > 0
				sess.TurnCount++
			}
		}
		completionID := "chatcmpl-" + sessionID
		created := time.Now().Unix()
		modelID := agent.Model
		if streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			flusher, canFlush := w.(http.Flusher)
			sendChunk := func(content string, finishReason *string) {
				chunk := ChatCompletionChunk{
					ID:      completionID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   modelID,
					Choices: []ChunkChoice{
						{
							Index:        0,
							Delta:        ChunkDelta{Content: content},
							FinishReason: finishReason,
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if canFlush {
					flusher.Flush()
				}
			}
			ctx := r.Context()
			runner(ctx, cfg, agent, sessionID, prompt, resume, func(eventType string, data []byte) error {
				switch eventType {
				case "assistant":
					ev, err := claude.ParseNDJSON(string(data))
					if err != nil {
						return nil
					}
					text, err := claude.ExtractText(ev)
					if err != nil || text == "" {
						return nil
					}
					sendChunk(text, nil)
				case "result":
					stop := "stop"
					sendChunk("", &stop)
				}
				return nil
			})
			fmt.Fprint(w, "data: [DONE]\n\n")
			if canFlush {
				flusher.Flush()
			}
		} else {
			var buf bytes.Buffer
			ctx := r.Context()
			runErr := runner(ctx, cfg, agent, sessionID, prompt, resume, func(eventType string, data []byte) error {
				if eventType == "result" {
					var resultEv struct {
						Result string `json:"result"`
					}
					if err := json.Unmarshal(data, &resultEv); err == nil {
						buf.WriteString(resultEv.Result)
					}
				}
				return nil
			})
			if runErr != nil {
				WriteError(w, http.StatusInternalServerError, runErr.Error())
				return
			}
			resp := OpenAIChatResponse{
				ID:      completionID,
				Object:  "chat.completion",
				Created: created,
				Model:   modelID,
				Choices: []OpenAIChoice{
					{
						Index: 0,
						Message: OpenAIMessage{
							Role:    "assistant",
							Content: buf.String(),
						},
						FinishReason: "stop",
					},
				},
				Usage: OpenAIUsage{},
			}
			WriteJSON(w, http.StatusOK, resp)
		}
	}
}

func RegisterOpenAIHandlers(
	mux *http.ServeMux,
	cfg domain.Config,
	st *store.Store,
	runner claude.Runner,
) {
	mux.HandleFunc("GET /v1/models", modelsHandler)
	mux.HandleFunc("POST /v1/chat/completions", openaiCompletionsHandler(cfg, st, runner))
}
