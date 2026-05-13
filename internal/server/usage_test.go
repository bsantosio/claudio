package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"claudio/internal/domain"
	"claudio/internal/server"
	"claudio/internal/store"
)

// newUsageTestServer creates a server with a runner that emits a result event
// with token usage data so the SSE handler saves a TokenUsage row.
func newUsageTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	cfg := newTestConfig(t)
	st, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	h := server.BuildMuxWithRunner(cfg, st, nil)
	return h, st
}

// seedUsage inserts token_usage rows directly via the store helper.
func seedUsage(t *testing.T, st *store.Store, sessionID, agentID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := st.SaveTokenUsage(domain.TokenUsage{
			SessionID:    sessionID,
			AgentID:      agentID,
			InputTokens:  100,
			OutputTokens: 50,
			TotalCostUSD: 0.001,
		})
		if err != nil {
			t.Fatalf("seedUsage[%d]: %v", i, err)
		}
	}
}

// ─── GET /sessions/{id}/usage ─────────────────────────────────────────────────

func TestGetSessionUsage_200_WithTurns(t *testing.T) {
	h, st := newUsageTestServer(t)
	a, _ := st.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(a.ID, "s1")
	seedUsage(t, st, sess.ID, a.ID, 3)

	w := getReq(t, h, "/sessions/"+sess.ID+"/usage")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var summary domain.UsageSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TurnCount != 3 {
		t.Errorf("expected TurnCount 3, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 300 {
		t.Errorf("expected TotalInputTokens 300, got %d", summary.TotalInputTokens)
	}
	if summary.TotalCostUSD != 0.003 {
		t.Errorf("expected TotalCostUSD 0.003, got %f", summary.TotalCostUSD)
	}
}

func TestGetSessionUsage_200_ZeroTurns(t *testing.T) {
	h, st := newUsageTestServer(t)
	a, _ := st.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(a.ID, "s1")
	// No usage rows seeded.

	w := getReq(t, h, "/sessions/"+sess.ID+"/usage")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var summary domain.UsageSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TurnCount != 0 {
		t.Errorf("expected TurnCount 0, got %d", summary.TurnCount)
	}
	if summary.TotalCostUSD != 0 {
		t.Errorf("expected TotalCostUSD 0, got %f", summary.TotalCostUSD)
	}
}

func TestGetSessionUsage_404_UnknownSession(t *testing.T) {
	h, _ := newUsageTestServer(t)
	w := getReq(t, h, "/sessions/does-not-exist/usage")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ─── GET /agents/{id}/usage ───────────────────────────────────────────────────

func TestGetAgentUsage_200_WithTurns(t *testing.T) {
	h, st := newUsageTestServer(t)
	a, _ := st.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess1, _ := st.CreateSession(a.ID, "s1")
	sess2, _ := st.CreateSession(a.ID, "s2")
	seedUsage(t, st, sess1.ID, a.ID, 2)
	seedUsage(t, st, sess2.ID, a.ID, 1)

	w := getReq(t, h, "/agents/"+a.ID+"/usage")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var summary domain.UsageSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TurnCount != 3 {
		t.Errorf("expected TurnCount 3, got %d", summary.TurnCount)
	}
}

func TestGetAgentUsage_404_UnknownAgent(t *testing.T) {
	h, _ := newUsageTestServer(t)
	w := getReq(t, h, "/agents/does-not-exist/usage")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ─── GET /usage ───────────────────────────────────────────────────────────────

func TestGetGlobalUsage_200_WithData(t *testing.T) {
	h, st := newUsageTestServer(t)
	a1, _ := st.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	a2, _ := st.CreateAgent(domain.Agent{Name: "a2", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess1, _ := st.CreateSession(a1.ID, "s1")
	sess2, _ := st.CreateSession(a2.ID, "s2")
	seedUsage(t, st, sess1.ID, a1.ID, 1)
	seedUsage(t, st, sess2.ID, a2.ID, 1)

	w := getReq(t, h, "/usage")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var summary domain.UsageSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TurnCount != 2 {
		t.Errorf("expected TurnCount 2, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 200 {
		t.Errorf("expected TotalInputTokens 200, got %d", summary.TotalInputTokens)
	}
}

func TestGetGlobalUsage_200_EmptyDatabase(t *testing.T) {
	h, _ := newUsageTestServer(t)
	w := getReq(t, h, "/usage")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var summary domain.UsageSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TurnCount != 0 {
		t.Errorf("expected TurnCount 0 for empty db, got %d", summary.TurnCount)
	}
}

func TestGetGlobalUsage_ReturnsAggregateNotRows(t *testing.T) {
	h, st := newUsageTestServer(t)
	a, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(a.ID, "s1")
	seedUsage(t, st, sess.ID, a.ID, 5)

	w := getReq(t, h, "/usage")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Response must be a single object (aggregate), not an array
	var body any
	json.NewDecoder(w.Body).Decode(&body) //nolint:errcheck
	if _, isSlice := body.([]any); isSlice {
		t.Error("expected aggregate object, got array — endpoint must not expose raw rows")
	}
}
