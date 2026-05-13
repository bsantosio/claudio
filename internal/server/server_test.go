package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/server"
	"claudio/internal/store"
)

func newTestConfig(t *testing.T) domain.Config {
	t.Helper()
	return domain.Config{
		Port:         "8080",
		DefaultModel: "sonnet",
		WorkDir:      t.TempDir(),
		DataDir:      t.TempDir(),
	}
}

func newTestServer(t *testing.T, cfg domain.Config) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	h := server.BuildMux(cfg, st)
	return h, st
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func putJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func deleteReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("DELETE", path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func boolPtr(b bool) *bool { return &b }

func newMockRunner(responseText string) claude.Runner {
	return func(ctx context.Context, cfg domain.Config, agent *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		assistantData := fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":%s}]},"session_id":"%s"}`,
			jsonString(responseText), sessionID)
		if err := onEvent("assistant", []byte(assistantData)); err != nil {
			return err
		}
		resultData := fmt.Sprintf(`{"type":"result","subtype":"success","result":%s,"session_id":"%s","total_cost_usd":0.001}`,
			jsonString(responseText), sessionID)
		return onEvent("result", []byte(resultData))
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// --- health endpoint tests ---

func TestHealthEndpoint_ReturnsStatusAndClaudeAuth(t *testing.T) {
	cfg := newTestConfig(t)
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp["status"]; !ok {
		t.Error("response missing 'status' field")
	}
	if _, ok := resp["claude_auth"]; !ok {
		t.Error("response missing 'claude_auth' field")
	}
}

func TestHealthEndpoint_ContentTypeIsJSON(t *testing.T) {
	cfg := newTestConfig(t)
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// --- API key middleware tests ---

func TestAPIKeyMiddleware_MissingKey_Returns401(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.APIKey = "secret"
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/agents", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_ValidKey_Passes(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.APIKey = "secret"
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/agents", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Error("expected request to pass auth, got 401")
	}
}

func TestAPIKeyMiddleware_HealthBypass(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.APIKey = "secret"
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected health to bypass auth (200), got %d", w.Code)
	}
}

func TestAPIKeyMiddleware_WrongKey_Returns401(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.APIKey = "secret"
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/agents", nil)
	req.Header.Set("Authorization", "Bearer wrongkey")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong key, got %d", w.Code)
	}
}

func TestCORS_OptionsReturns204(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("OPTIONS", "/agents", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected CORS origin *, got %q", got)
	}
}

func TestCORS_HeadersOnRegularRequest(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected CORS origin * on regular request, got %q", got)
	}
}

func TestAPIKeyMiddleware_NoAPIKeySet_AllowsAll(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.APIKey = ""
	h := server.BuildMux(cfg, nil)
	req := httptest.NewRequest("GET", "/agents", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Error("expected no auth enforcement when API_KEY not set")
	}
}

// --- Agent CRUD tests ---

func TestAgentCreate_Valid(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{
		"name": "test-agent", "system_prompt": "You are helpful.", "model": "sonnet",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var agent domain.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.ID == "" {
		t.Error("agent.id should be set")
	}
	if agent.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", agent.Name)
	}
}

func TestAgentCreate_MissingName(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"system_prompt": "sp", "model": "sonnet"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAgentCreate_MissingModel_UsesDefault(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.DefaultModel = "sonnet"
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"name": "test-agent", "system_prompt": "sp"})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var agent domain.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	if agent.Model != "sonnet" {
		t.Errorf("expected model 'sonnet', got %q", agent.Model)
	}
}

func TestAgentCreate_InvalidModel(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"name": "a", "system_prompt": "sp", "model": "gpt-4o"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAgentGet_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := getReq(t, h, "/agents/nonexistent-id")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAgentList(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	postJSON(t, h, "/agents", map[string]any{"name": "a1", "system_prompt": "sp1", "model": "sonnet"})
	postJSON(t, h, "/agents", map[string]any{"name": "a2", "system_prompt": "sp2", "model": "haiku"})
	w := getReq(t, h, "/agents")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var agents []domain.Agent
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestAgentUpdate(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"name": "original", "system_prompt": "original prompt", "model": "sonnet"})
	var agent domain.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	time.Sleep(2 * time.Millisecond)
	w2 := putJSON(t, h, "/agents/"+agent.ID, map[string]any{"name": "updated", "system_prompt": "new prompt", "model": "sonnet"})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var updated domain.Agent
	json.NewDecoder(w2.Body).Decode(&updated)
	if updated.Name != "updated" {
		t.Errorf("expected name 'updated', got %q", updated.Name)
	}
}

func TestAgentDelete_NoSessions(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"name": "to-delete", "system_prompt": "sp", "model": "sonnet"})
	var agent domain.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	w2 := deleteReq(t, h, "/agents/"+agent.ID)
	if w2.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestAgentDelete_WithActiveSessions_Returns409(t *testing.T) {
	cfg := newTestConfig(t)
	h, st := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"name": "busy", "system_prompt": "sp", "model": "sonnet"})
	var agent domain.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	st.CreateSession(agent.ID, "test-session")
	w2 := deleteReq(t, h, "/agents/"+agent.ID)
	if w2.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w2.Code)
	}
}

// --- Session HTTP tests ---

func TestSessionCreate_ValidAgent(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents", map[string]any{"name": "my-agent", "system_prompt": "sp", "model": "sonnet"})
	var agent domain.Agent
	json.NewDecoder(w.Body).Decode(&agent)
	w2 := postJSON(t, h, "/agents/"+agent.ID+"/sessions", map[string]any{"name": "my-session"})
	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestSessionCreate_NonExistentAgent_Returns404(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := postJSON(t, h, "/agents/does-not-exist/sessions", map[string]any{})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSessionList_FilteredByAgent(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w1 := postJSON(t, h, "/agents", map[string]any{"name": "a1", "system_prompt": "sp", "model": "sonnet"})
	var a1 domain.Agent
	json.NewDecoder(w1.Body).Decode(&a1)
	w2 := postJSON(t, h, "/agents", map[string]any{"name": "a2", "system_prompt": "sp", "model": "sonnet"})
	var a2 domain.Agent
	json.NewDecoder(w2.Body).Decode(&a2)
	postJSON(t, h, "/agents/"+a1.ID+"/sessions", map[string]any{"name": "s1"})
	postJSON(t, h, "/agents/"+a1.ID+"/sessions", map[string]any{"name": "s2"})
	postJSON(t, h, "/agents/"+a2.ID+"/sessions", map[string]any{"name": "s3"})
	w := getReq(t, h, "/agents/"+a1.ID+"/sessions")
	var sessions []domain.Session
	json.NewDecoder(w.Body).Decode(&sessions)
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionGet_NotFound(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w := getReq(t, h, "/sessions/nonexistent-id")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSessionDelete_Idle(t *testing.T) {
	cfg := newTestConfig(t)
	h, _ := newTestServer(t, cfg)
	w1 := postJSON(t, h, "/agents", map[string]any{"name": "a1", "system_prompt": "sp", "model": "sonnet"})
	var agent domain.Agent
	json.NewDecoder(w1.Body).Decode(&agent)
	w2 := postJSON(t, h, "/agents/"+agent.ID+"/sessions", map[string]any{})
	var sess domain.Session
	json.NewDecoder(w2.Body).Decode(&sess)
	w3 := deleteReq(t, h, "/sessions/"+sess.ID)
	if w3.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w3.Code, w3.Body.String())
	}
}

// --- UUID tests ---

func TestNewUUID_IsValidV4(t *testing.T) {
	for i := 0; i < 100; i++ {
		id, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("NewUUID() error: %v", err)
		}
		if len(id) != 36 {
			t.Errorf("invalid UUID length: %q", id)
		}
	}
}

// --- Message handler tests ---

func TestMessageHandler_SessionNotFound(t *testing.T) {
	cfg := newTestConfig(t)
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mux := server.BuildMuxWithRunner(cfg, st, nil)
	body := strings.NewReader(`{"content":"Hello"}`)
	req := httptest.NewRequest("POST", "/sessions/nonexistent-session-id/message", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMessageHandler_SuccessfulStream(t *testing.T) {
	cfg := newTestConfig(t)
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	agent, _ := st.CreateAgent(domain.Agent{Name: "test-agent", SystemPrompt: "You are helpful.", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "test-session")
	mockRunner := func(ctx context.Context, cfg domain.Config, a *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		textData := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hi there!"}]},"session_id":"` + sessionID + `"}`
		if err := onEvent("assistant", []byte(textData)); err != nil {
			return err
		}
		resultData := `{"type":"result","subtype":"success","result":"Hi there!","session_id":"` + sessionID + `","total_cost_usd":0.001}`
		return onEvent("result", []byte(resultData))
	}
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)
	body := strings.NewReader(`{"content":"Hello"}`)
	req := httptest.NewRequest("POST", "/sessions/"+sess.ID+"/message", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

func TestMessageHandler_EmptyContent(t *testing.T) {
	cfg := newTestConfig(t)
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	agent, _ := st.CreateAgent(domain.Agent{Name: "test", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "")
	mux := server.BuildMuxWithRunner(cfg, st, nil)
	body := strings.NewReader(`{"content":""}`)
	req := httptest.NewRequest("POST", "/sessions/"+sess.ID+"/message", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- OpenAI tests ---

func TestNormalizeModel_GPT4MapsToOpus(t *testing.T) {
	result := server.NormalizeModel("gpt-4")
	if result != "claude-opus-4-5" {
		t.Errorf("expected 'claude-opus-4-5', got %q", result)
	}
}

func TestNormalizeModel_UnknownPassthrough(t *testing.T) {
	result := server.NormalizeModel("some-future-model")
	if result != "some-future-model" {
		t.Errorf("expected unknown model to pass through, got %q", result)
	}
}

func TestModelsHandler_ReturnsModelList(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet"}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mux := server.BuildMuxWithRunner(cfg, st, nil)
	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %q", resp.Object)
	}
	if len(resp.Data) < 3 {
		t.Errorf("expected at least 3 models, got %d", len(resp.Data))
	}
}

func TestChatCompletions_NonStreaming(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mockRunner := func(ctx context.Context, c domain.Config, a *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		resultData := `{"type":"result","subtype":"success","result":"Paris is the capital.","session_id":"` + sessionID + `","total_cost_usd":0.001}`
		return onEvent("result", []byte(resultData))
	}
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)
	reqBody := server.OpenAIChatRequest{
		Model:    "gpt-4",
		Messages: []server.OpenAIMessage{{Role: "user", Content: "What is the capital of France?"}},
		Stream:   boolPtr(false),
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp server.OpenAIChatResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", resp.Object)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.Choices[0].FinishReason)
	}
}

func TestChatCompletions_Streaming(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mockRunner := func(ctx context.Context, c domain.Config, a *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		assistantData := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]},"session_id":"` + sessionID + `"}`
		onEvent("assistant", []byte(assistantData))
		resultData := `{"type":"result","subtype":"success","result":"Hello","session_id":"` + sessionID + `","total_cost_usd":0.001}`
		return onEvent("result", []byte(resultData))
	}
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)
	reqBody := server.OpenAIChatRequest{
		Model:    "gpt-4o",
		Messages: []server.OpenAIMessage{{Role: "user", Content: "Hi"}},
		Stream:   boolPtr(true),
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("expected 'data: [DONE]' in SSE stream")
	}
	if !strings.Contains(body, `"delta"`) {
		t.Errorf("expected delta chunks in SSE stream")
	}
}

// --- Integration tests ---

func TestIntegration_FullLifecycle(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mockRunner := newMockRunner("Hello from Claude!")
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)

	// Create agent
	agentBody, _ := json.Marshal(map[string]string{"name": "integration-agent", "system_prompt": "You are a test assistant.", "model": "sonnet"})
	req := httptest.NewRequest("POST", "/agents", bytes.NewReader(agentBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create agent: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var createdAgent domain.Agent
	json.NewDecoder(w.Body).Decode(&createdAgent)

	// Create session
	sessBody, _ := json.Marshal(map[string]string{"name": "integration-session"})
	req = httptest.NewRequest("POST", "/agents/"+createdAgent.ID+"/sessions", bytes.NewReader(sessBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create session: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var createdSession domain.Session
	json.NewDecoder(w.Body).Decode(&createdSession)

	// Send message
	msgBody := strings.NewReader(`{"content":"Hello!"}`)
	req = httptest.NewRequest("POST", "/sessions/"+createdSession.ID+"/message", msgBody)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("message: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete session
	req = httptest.NewRequest("DELETE", "/sessions/"+createdSession.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete session: expected 204, got %d", w.Code)
	}

	// Delete agent
	req = httptest.NewRequest("DELETE", "/agents/"+createdAgent.ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete agent: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIntegration_OpenAI_WithAgentId(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	agent, _ := st.CreateAgent(domain.Agent{Name: "specialized", SystemPrompt: "You are a specialized expert.", Model: "claude-opus-4-5"}, "sonnet")
	var capturedSystemPrompt string
	mockRunner := func(ctx context.Context, cfg domain.Config, a *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		capturedSystemPrompt = a.SystemPrompt
		resultData := fmt.Sprintf(`{"type":"result","subtype":"success","result":"expert answer","session_id":"%s","total_cost_usd":0}`, sessionID)
		return onEvent("result", []byte(resultData))
	}
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)
	reqBody := server.OpenAIChatRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []server.OpenAIMessage{{Role: "user", Content: "Question"}},
		Stream:   boolPtr(false),
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Agent-Id", agent.ID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedSystemPrompt != "You are a specialized expert." {
		t.Errorf("expected agent system prompt, got %q", capturedSystemPrompt)
	}
}

func TestIntegration_APIKeyProtection(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", APIKey: "test-secret", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mux := server.BuildMuxWithRunner(cfg, st, nil)

	protectedRoutes := []struct{ method, path string }{
		{"GET", "/agents"},
		{"GET", "/v1/models"},
		{"POST", "/v1/chat/completions"},
	}
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without auth for %s %s, got %d", route.method, route.path, w.Code)
			}
		})
	}
}

func TestChatCompletions_Streaming_ChunkFormat(t *testing.T) {
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir(), DataDir: t.TempDir()}
	st, _ := store.NewStore(":memory:")
	defer st.Close()
	mockRunner := func(ctx context.Context, c domain.Config, a *domain.Agent, sessionID string, message string, resume bool, onEvent claude.StreamCallback) error {
		assistantData := `{"type":"assistant","message":{"content":[{"type":"text","text":"World"}]},"session_id":"` + sessionID + `"}`
		onEvent("assistant", []byte(assistantData))
		resultData := `{"type":"result","subtype":"success","result":"World","session_id":"` + sessionID + `","total_cost_usd":0}`
		return onEvent("result", []byte(resultData))
	}
	mux := server.BuildMuxWithRunner(cfg, st, mockRunner)
	reqBody := server.OpenAIChatRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []server.OpenAIMessage{{Role: "user", Content: "say world"}},
		Stream:   boolPtr(true),
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var chunks []server.ChatCompletionChunk
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var chunk server.ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("failed to parse chunk: %v", err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	for _, chunk := range chunks {
		if chunk.Object != "chat.completion.chunk" {
			t.Errorf("expected object 'chat.completion.chunk', got %q", chunk.Object)
		}
	}
}
