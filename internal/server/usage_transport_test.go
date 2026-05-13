package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/server"
	"claudio/internal/store"
)

// newMockRunnerWithUsage returns a runner that emits a result event with
// token usage fields populated. It is used to verify transport persistence.
func newMockRunnerWithUsage(responseText string) claude.Runner {
	return func(ctx context.Context, cfg domain.Config, agent *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		assistantData := `{"type":"assistant","message":{"content":[{"type":"text","text":` +
			jsonString(responseText) + `}]},"session_id":"` + sessionID + `"}`
		if err := onEvent("assistant", []byte(assistantData)); err != nil {
			return err
		}
		resultData := `{"type":"result","subtype":"success","result":` +
			jsonString(responseText) +
			`,"session_id":"` + sessionID + `","total_cost_usd":0.002,` +
			`"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}`
		return onEvent("result", []byte(resultData))
	}
}

// ─── HTTP SSE handler (P1) ────────────────────────────────────────────────────

func TestHTTPHandler_PersistsTokenUsageAfterResultEvent(t *testing.T) {
	cfg := newTestConfig(t)
	st, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()

	agent, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "s1")

	mockRunner := newMockRunnerWithUsage("hello")
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)

	body := strings.NewReader(`{"content":"hi"}`)
	req := httptest.NewRequest("POST", "/sessions/"+sess.ID+"/message", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("handler failed: %d — %s", w.Code, w.Body.String())
	}

	// Verify usage was persisted
	summary, err := st.GetSessionUsage(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionUsage: %v", err)
	}
	if summary.TurnCount != 1 {
		t.Errorf("expected TurnCount 1 after message, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 100 {
		t.Errorf("expected TotalInputTokens 100, got %d", summary.TotalInputTokens)
	}
	if summary.TotalCostUSD != 0.002 {
		t.Errorf("expected TotalCostUSD 0.002, got %f", summary.TotalCostUSD)
	}
}

func TestHTTPHandler_UsagePersistenceNonFatal(t *testing.T) {
	// Verify that even if we send a result event without token data, the SSE
	// stream still completes (P6 — persistence failure does not block response).
	cfg := newTestConfig(t)
	st, _ := store.NewStore(":memory:")
	defer st.Close()

	agent, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "s1")

	minimalRunner := func(ctx context.Context, c domain.Config, a *domain.Agent, sid string, msg string, resume bool, onEvent claude.StreamCallback) error {
		// Emit result event with no usage fields
		resultData := `{"type":"result","subtype":"success","result":"ok","session_id":"` + sid + `","total_cost_usd":0}`
		return onEvent("result", []byte(resultData))
	}
	mux := server.BuildMuxWithRunner(cfg, st, minimalRunner)

	body := strings.NewReader(`{"content":"hi"}`)
	req := httptest.NewRequest("POST", "/sessions/"+sess.ID+"/message", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 even with zero usage data, got %d", w.Code)
	}
}

// ─── OpenAI streaming handler (P5) ───────────────────────────────────────────

func TestOpenAIStream_PersistsTokenUsageAndWritesDone(t *testing.T) {
	// OpenAI streaming with a session-backed request (X-Session-Id header)
	// so the FK for session_id is satisfied.
	cfg := newTestConfig(t)
	st, _ := store.NewStore(":memory:")
	defer st.Close()

	agent, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "s1")

	mockRunner := newMockRunnerWithUsage("stream-response")
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)

	reqBody := server.OpenAIChatRequest{
		Model:    "gpt-4o",
		Messages: []server.OpenAIMessage{{Role: "user", Content: "Hello"}},
		Stream:   boolPtr(true),
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Agent-Id", agent.ID)
	req.Header.Set("X-Session-Id", sess.ID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// P5: [DONE] MUST be written after usage persistence
	if !strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Error("expected [DONE] in stream after usage persistence")
	}
	// Verify usage was persisted via the session
	summary, err := st.GetSessionUsage(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionUsage: %v", err)
	}
	if summary.TurnCount != 1 {
		t.Errorf("expected TurnCount 1 after OpenAI stream, got %d", summary.TurnCount)
	}
}
