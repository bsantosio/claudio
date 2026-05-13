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
