package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
)

type OpenAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      *bool           `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
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
	"gpt-4":         "claude-opus-4-7",
	"gpt-4-turbo":   "claude-opus-4-7",
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
	{ID: "claude-opus-4-6", Object: "model", Created: 1714000002, OwnedBy: "anthropic"},
	{ID: "claude-opus-4-6[1m]", Object: "model", Created: 1714000003, OwnedBy: "anthropic"},
	{ID: "claude-opus-4-7", Object: "model", Created: 1714000004, OwnedBy: "anthropic"},
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"object": "list",
		"data":   knownClaudeModels,
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) registerOpenAIHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/models", modelsHandler)
	mux.HandleFunc("POST /v1/chat/completions", s.openaiCompletionsHandler())
}

func (s *Server) openaiCompletionsHandler() http.HandlerFunc {
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
			a, err := s.store.GetAgent(agentID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					WriteError(w, http.StatusNotFound, "agent not found")
				} else {
					WriteError(w, http.StatusInternalServerError, err.Error())
				}
				return
			}
			agent = a
		} else {
			model := NormalizeModel(req.Model)
			if model == "" {
				model = s.cfg.DefaultModel
			}
			if !domain.KnownModels[model] {
				model = s.cfg.DefaultModel
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
		var promptParts []string
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				continue
			}
			promptParts = append(promptParts, fmt.Sprintf("[%s]: %s", msg.Role, msg.Content))
		}
		prompt := strings.Join(promptParts, "\n\n")
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
		var activeSession *domain.Session
		if sid := r.Header.Get("X-Session-Id"); sid != "" {
			if sess, err := s.store.GetSession(sid); err == nil {
				sessionID = sess.ID
				resume = sess.TurnCount > 0
				sess.TurnCount++
				activeSession = sess
			}
		}
		completionID := "chatcmpl-" + sessionID
		created := time.Now().Unix()

		streaming := req.Stream != nil && *req.Stream
		if streaming {
			s.handleOpenAIStream(w, r, agent, sessionID, prompt, resume, completionID, created)
		} else {
			s.handleOpenAISync(w, r, agent, sessionID, prompt, resume, completionID, created)
		}
		if activeSession != nil {
			if err := s.store.SaveSession(activeSession); err != nil {
				log.Printf("failed to save openai session: %v", err)
			}
		}
	}
}

func (s *Server) handleOpenAIStream(w http.ResponseWriter, r *http.Request, agent *domain.Agent, sessionID, prompt string, resume bool, completionID string, created int64) {
	modelID := agent.Model
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
	err := s.runner(ctx, s.cfg, agent, sessionID, prompt, resume, func(eventType string, data []byte) error {
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
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", errJSON)
		if canFlush {
			flusher.Flush()
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

func (s *Server) handleOpenAISync(w http.ResponseWriter, r *http.Request, agent *domain.Agent, sessionID, prompt string, resume bool, completionID string, created int64) {
	modelID := agent.Model
	var buf bytes.Buffer
	var usage OpenAIUsage
	ctx := r.Context()
	runErr := s.runner(ctx, s.cfg, agent, sessionID, prompt, resume, func(eventType string, data []byte) error {
		if eventType == "result" {
			var resultEv struct {
				Result string `json:"result"`
				Usage  struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(data, &resultEv); err == nil {
				buf.WriteString(resultEv.Result)
				usage.PromptTokens = resultEv.Usage.InputTokens + resultEv.Usage.CacheReadInputTokens + resultEv.Usage.CacheCreationInputTokens
				usage.CompletionTokens = resultEv.Usage.OutputTokens
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
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
		Usage: usage,
	}
	WriteJSON(w, http.StatusOK, resp)
}
