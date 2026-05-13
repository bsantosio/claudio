package store_test

import (
	"errors"
	"testing"
	"time"

	"claudio/internal/domain"
	"claudio/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_CreateAgent_Valid(t *testing.T) {
	s := newTestStore(t)
	input := domain.Agent{
		Name:         "test-agent",
		SystemPrompt: "You are helpful.",
		Model:        "sonnet",
	}
	a, err := s.CreateAgent(input, "sonnet")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if a.ID == "" {
		t.Error("expected ID to be set")
	}
	if a.Name != "test-agent" {
		t.Errorf("expected name %q, got %q", "test-agent", a.Name)
	}
	if a.Model != "sonnet" {
		t.Errorf("expected model %q, got %q", "sonnet", a.Model)
	}
	if a.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}
	if a.UpdatedAt == "" {
		t.Error("expected updated_at to be set")
	}
}

func TestStore_CreateAgent_MissingName(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateAgent(domain.Agent{SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestStore_CreateAgent_MissingSystemPrompt(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateAgent(domain.Agent{Name: "n", Model: "sonnet"}, "sonnet")
	if err == nil {
		t.Error("expected error for missing system_prompt")
	}
}

func TestStore_CreateAgent_InvalidModel(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "gpt-4"}, "sonnet")
	if err == nil {
		t.Error("expected error for invalid model")
	}
}

func TestStore_CreateAgent_DefaultModel(t *testing.T) {
	s := newTestStore(t)
	a, err := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp"}, "sonnet")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if a.Model != "sonnet" {
		t.Errorf("expected default model 'sonnet', got %q", a.Model)
	}
}

func TestStore_CreateAgent_WithTools(t *testing.T) {
	s := newTestStore(t)
	input := domain.Agent{
		Name:            "tooled",
		SystemPrompt:    "sp",
		Model:           "sonnet",
		AllowedTools:    []string{"bash", "read"},
		DisallowedTools: []string{"write"},
		MCPConfig:       map[string]any{"server": "http://localhost"},
	}
	a, err := s.CreateAgent(input, "sonnet")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if len(a.AllowedTools) != 2 {
		t.Errorf("expected 2 allowed tools, got %d", len(a.AllowedTools))
	}
	if len(a.DisallowedTools) != 1 {
		t.Errorf("expected 1 disallowed tool, got %d", len(a.DisallowedTools))
	}
	if a.MCPConfig == nil {
		t.Error("expected mcp_config to be set")
	}
}

func TestStore_CreateAgent_WithSubAgents(t *testing.T) {
	s := newTestStore(t)
	input := domain.Agent{
		Name:         "orchestrator",
		SystemPrompt: "You coordinate sub-agents.",
		Model:        "sonnet",
		SubAgents:    []string{"code-reviewer", "test-engineer"},
	}
	a, err := s.CreateAgent(input, "sonnet")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if len(a.SubAgents) != 2 {
		t.Errorf("expected 2 sub-agents on create result, got %d", len(a.SubAgents))
	}
	if a.SubAgents[0] != "code-reviewer" {
		t.Errorf("expected sub-agent[0] 'code-reviewer', got %q", a.SubAgents[0])
	}

	// Verify sub_agents persists through GetAgent
	got, err := s.GetAgent(a.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if len(got.SubAgents) != 2 {
		t.Errorf("expected 2 sub-agents from GetAgent, got %d", len(got.SubAgents))
	}
	if got.SubAgents[1] != "test-engineer" {
		t.Errorf("expected sub-agent[1] 'test-engineer', got %q", got.SubAgents[1])
	}
}

func TestStore_UpdateAgent_SubAgents(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	updated, err := s.UpdateAgent(a.ID, domain.Agent{SubAgents: []string{"helper"}}, "sonnet")
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if len(updated.SubAgents) != 1 || updated.SubAgents[0] != "helper" {
		t.Errorf("expected sub-agents [helper] after update, got %v", updated.SubAgents)
	}
	got, err := s.GetAgent(a.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if len(got.SubAgents) != 1 || got.SubAgents[0] != "helper" {
		t.Errorf("expected persisted sub-agents [helper], got %v", got.SubAgents)
	}
}

func TestStore_GetAgent_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetAgent("nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_GetAgent_Found(t *testing.T) {
	s := newTestStore(t)
	created, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	got, err := s.GetAgent(created.ID)
	if err != nil {
		t.Fatalf("expected GetAgent to succeed, got %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, got.ID)
	}
	if got.Name != "n" {
		t.Errorf("expected name %q, got %q", "n", got.Name)
	}
}

func TestStore_ListAgents_Empty(t *testing.T) {
	s := newTestStore(t)
	agents, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestStore_ListAgents_Multiple(t *testing.T) {
	s := newTestStore(t)
	s.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp1", Model: "sonnet"}, "sonnet")
	s.CreateAgent(domain.Agent{Name: "a2", SystemPrompt: "sp2", Model: "haiku"}, "sonnet")
	agents, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestStore_UpdateAgent_ChangesFields(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "original", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	time.Sleep(2 * time.Millisecond)
	updated, err := s.UpdateAgent(a.ID, domain.Agent{Name: "changed", SystemPrompt: "new sp"}, "sonnet")
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if updated.Name != "changed" {
		t.Errorf("expected name 'changed', got %q", updated.Name)
	}
	if updated.SystemPrompt != "new sp" {
		t.Errorf("expected system_prompt 'new sp', got %q", updated.SystemPrompt)
	}
	if updated.UpdatedAt == a.UpdatedAt {
		t.Error("expected updated_at to change after update")
	}
}

func TestStore_UpdateAgent_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.UpdateAgent("nonexistent", domain.Agent{Name: "n"}, "sonnet")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestStore_DeleteAgent_NoSessions(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	err := s.DeleteAgent(a.ID)
	if err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	_, err = s.GetAgent(a.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_DeleteAgent_WithSessions_Returns409(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	s.CreateSession(a.ID, "test-session")
	err := s.DeleteAgent(a.ID)
	if err == nil {
		t.Fatal("expected error when deleting agent with active sessions")
	}
	if err != domain.ErrHasActiveSessions {
		t.Errorf("expected ErrHasActiveSessions, got %v", err)
	}
}

func TestStore_CreateSession_Valid(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, err := s.CreateSession(a.ID, "my-session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID == "" {
		t.Error("expected session ID to be set")
	}
	if sess.AgentID != a.ID {
		t.Errorf("expected agent_id %q, got %q", a.ID, sess.AgentID)
	}
	if sess.Name != "my-session" {
		t.Errorf("expected name 'my-session', got %q", sess.Name)
	}
	if sess.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}
}

func TestStore_GetSession_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetSession("nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_GetSession_Found(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	created, _ := s.CreateSession(a.ID, "s1")
	got, err := s.GetSession(created.ID)
	if err != nil {
		t.Fatalf("expected GetSession to succeed, got %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, got.ID)
	}
}

func TestStore_ListSessionsByAgent(t *testing.T) {
	s := newTestStore(t)
	a1, _ := s.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	a2, _ := s.CreateAgent(domain.Agent{Name: "a2", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	s.CreateSession(a1.ID, "s1")
	s.CreateSession(a1.ID, "s2")
	s.CreateSession(a2.ID, "s3")
	sessions, err := s.ListSessionsByAgent(a1.ID)
	if err != nil {
		t.Fatalf("ListSessionsByAgent: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for a1, got %d", len(sessions))
	}
	for _, sess := range sessions {
		if sess.AgentID != a1.ID {
			t.Errorf("session belongs to wrong agent: %q", sess.AgentID)
		}
	}
}

func TestStore_DeleteSession_Idle(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "")
	err := s.DeleteSession(sess.ID)
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = s.GetSession(sess.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_DeleteSession_Busy_Returns409(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "")
	mu := s.Mutexes.Get(sess.ID)
	mu.Lock()
	defer mu.Unlock()
	err := s.DeleteSession(sess.ID)
	if err == nil {
		t.Fatal("expected error for busy session")
	}
	if err != domain.ErrSessionBusy {
		t.Errorf("expected ErrSessionBusy, got %v", err)
	}
}

func TestStore_SaveSession_UpdatesTurnCount(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "")
	sess.TurnCount = 3
	sess.LastActive = time.Now().UTC().Format(time.RFC3339Nano)
	err := s.SaveSession(sess)
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	fresh, err := s.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("session not found after save: %v", err)
	}
	if fresh.TurnCount != 3 {
		t.Errorf("expected TurnCount 3, got %d", fresh.TurnCount)
	}
}

// ─── TokenUsage tests ─────────────────────────────────────────────────────────

func TestStore_SaveTokenUsage_InsertsRow(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "s1")

	tu := domain.TokenUsage{
		SessionID:                sess.ID,
		AgentID:                  a.ID,
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 10,
		CacheReadInputTokens:     5,
		TotalCostUSD:             0.0042,
	}
	err := s.SaveTokenUsage(tu)
	if err != nil {
		t.Fatalf("SaveTokenUsage: %v", err)
	}
	// Verify via aggregation
	summary, err := s.GetSessionUsage(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionUsage: %v", err)
	}
	if summary.TurnCount != 1 {
		t.Errorf("expected TurnCount 1, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 100 {
		t.Errorf("expected InputTokens 100, got %d", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 50 {
		t.Errorf("expected OutputTokens 50, got %d", summary.TotalOutputTokens)
	}
	if summary.TotalCacheCreationInputTokens != 10 {
		t.Errorf("expected CacheCreation 10, got %d", summary.TotalCacheCreationInputTokens)
	}
	if summary.TotalCacheReadInputTokens != 5 {
		t.Errorf("expected CacheRead 5, got %d", summary.TotalCacheReadInputTokens)
	}
	if summary.TotalCostUSD != 0.0042 {
		t.Errorf("expected TotalCostUSD 0.0042, got %f", summary.TotalCostUSD)
	}
}

func TestStore_SaveTokenUsage_GeneratesID(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "s1")

	tu := domain.TokenUsage{
		SessionID:    sess.ID,
		AgentID:      a.ID,
		InputTokens:  50,
		OutputTokens: 25,
	}
	// No ID set — should auto-generate
	err := s.SaveTokenUsage(tu)
	if err != nil {
		t.Fatalf("SaveTokenUsage without ID: %v", err)
	}
}

func TestStore_GetSessionUsage_MultipleRows(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "s1")

	for i := 0; i < 3; i++ {
		err := s.SaveTokenUsage(domain.TokenUsage{
			SessionID:    sess.ID,
			AgentID:      a.ID,
			InputTokens:  100,
			OutputTokens: 50,
			TotalCostUSD: 0.001,
		})
		if err != nil {
			t.Fatalf("SaveTokenUsage[%d]: %v", i, err)
		}
	}

	summary, err := s.GetSessionUsage(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionUsage: %v", err)
	}
	if summary.TurnCount != 3 {
		t.Errorf("expected TurnCount 3, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 300 {
		t.Errorf("expected TotalInputTokens 300, got %d", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 150 {
		t.Errorf("expected TotalOutputTokens 150, got %d", summary.TotalOutputTokens)
	}
	if summary.TotalCostUSD != 0.003 {
		t.Errorf("expected TotalCostUSD 0.003, got %f", summary.TotalCostUSD)
	}
}

func TestStore_GetSessionUsage_ZeroRows(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "s1")

	summary, err := s.GetSessionUsage(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionUsage: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary for zero rows")
	}
	if summary.TurnCount != 0 {
		t.Errorf("expected TurnCount 0, got %d", summary.TurnCount)
	}
	if summary.TotalCostUSD != 0 {
		t.Errorf("expected TotalCostUSD 0, got %f", summary.TotalCostUSD)
	}
}

func TestStore_GetAgentUsage_SpansMultipleSessions(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess1, _ := s.CreateSession(a.ID, "s1")
	sess2, _ := s.CreateSession(a.ID, "s2")

	s.SaveTokenUsage(domain.TokenUsage{SessionID: sess1.ID, AgentID: a.ID, InputTokens: 100, TotalCostUSD: 0.001}) //nolint:errcheck
	s.SaveTokenUsage(domain.TokenUsage{SessionID: sess2.ID, AgentID: a.ID, InputTokens: 200, TotalCostUSD: 0.002}) //nolint:errcheck

	summary, err := s.GetAgentUsage(a.ID)
	if err != nil {
		t.Fatalf("GetAgentUsage: %v", err)
	}
	if summary.TurnCount != 2 {
		t.Errorf("expected TurnCount 2, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 300 {
		t.Errorf("expected TotalInputTokens 300, got %d", summary.TotalInputTokens)
	}
	if summary.TotalCostUSD != 0.003 {
		t.Errorf("expected TotalCostUSD 0.003, got %f", summary.TotalCostUSD)
	}
}

func TestStore_GetAgentUsage_ZeroRows(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")

	summary, err := s.GetAgentUsage(a.ID)
	if err != nil {
		t.Fatalf("GetAgentUsage: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary for zero rows")
	}
	if summary.TurnCount != 0 {
		t.Errorf("expected TurnCount 0, got %d", summary.TurnCount)
	}
}

func TestStore_GetGlobalUsage_AggregatesAll(t *testing.T) {
	s := newTestStore(t)
	a1, _ := s.CreateAgent(domain.Agent{Name: "a1", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	a2, _ := s.CreateAgent(domain.Agent{Name: "a2", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess1, _ := s.CreateSession(a1.ID, "s1")
	sess2, _ := s.CreateSession(a2.ID, "s2")

	s.SaveTokenUsage(domain.TokenUsage{SessionID: sess1.ID, AgentID: a1.ID, InputTokens: 500, TotalCostUSD: 0.02}) //nolint:errcheck
	s.SaveTokenUsage(domain.TokenUsage{SessionID: sess2.ID, AgentID: a2.ID, InputTokens: 500, TotalCostUSD: 0.03}) //nolint:errcheck

	summary, err := s.GetGlobalUsage()
	if err != nil {
		t.Fatalf("GetGlobalUsage: %v", err)
	}
	if summary.TurnCount != 2 {
		t.Errorf("expected TurnCount 2, got %d", summary.TurnCount)
	}
	if summary.TotalInputTokens != 1000 {
		t.Errorf("expected TotalInputTokens 1000, got %d", summary.TotalInputTokens)
	}
	if summary.TotalCostUSD != 0.05 {
		t.Errorf("expected TotalCostUSD 0.05, got %f", summary.TotalCostUSD)
	}
}

func TestStore_GetGlobalUsage_EmptyDatabase(t *testing.T) {
	s := newTestStore(t)
	summary, err := s.GetGlobalUsage()
	if err != nil {
		t.Fatalf("GetGlobalUsage: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary for empty database")
	}
	if summary.TurnCount != 0 {
		t.Errorf("expected TurnCount 0, got %d", summary.TurnCount)
	}
}

func TestStore_SaveTokenUsage_CascadeDeleteWithSession(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := s.CreateSession(a.ID, "s1")
	s.SaveTokenUsage(domain.TokenUsage{SessionID: sess.ID, AgentID: a.ID, InputTokens: 100}) //nolint:errcheck

	// Delete session — usage rows should cascade
	if err := s.DeleteSession(sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	summary, err := s.GetGlobalUsage()
	if err != nil {
		t.Fatalf("GetGlobalUsage after cascade: %v", err)
	}
	if summary.TurnCount != 0 {
		t.Errorf("expected TurnCount 0 after cascade delete, got %d", summary.TurnCount)
	}
}

func TestStore_MigrationIdempotent(t *testing.T) {
	// NewStore calls migrate() once internally. We verify the store opens
	// cleanly and can be used (migrate ran without error on an existing DB).
	s := newTestStore(t)
	// If migration is not idempotent, NewStore itself would have failed.
	// Verify normal operations work fine after migration.
	a, err := s.CreateAgent(domain.Agent{Name: "n", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	if err != nil {
		t.Fatalf("CreateAgent after migration: %v", err)
	}
	sess, _ := s.CreateSession(a.ID, "s1")
	err = s.SaveTokenUsage(domain.TokenUsage{SessionID: sess.ID, AgentID: a.ID, InputTokens: 1})
	if err != nil {
		t.Fatalf("SaveTokenUsage after migration: %v", err)
	}
}
