package tui

import (
	"fmt"
	"testing"

	"claudio/internal/domain"
	"claudio/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	st, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := domain.Config{
		Port:         "18080",
		DefaultModel: "sonnet",
		WorkDir:      t.TempDir(),
		DataDir:      t.TempDir(),
	}
	return NewModel(cfg, st)
}

// TestNewModel_InitialState verifies the model starts on screenMenu with a
// zeroed cursor and no waiting state.
func TestNewModel_InitialState(t *testing.T) {
	m := newTestModel(t)
	if m.screen != screenMenu {
		t.Errorf("initial screen = %v, want screenMenu", m.screen)
	}
	if m.cursor != 0 {
		t.Error("initial cursor should be 0")
	}
	if m.waiting {
		t.Error("should not be waiting initially")
	}
}

// TestUpdate_WindowResize verifies that tea.WindowSizeMsg updates width/height.
func TestUpdate_WindowResize(t *testing.T) {
	m := newTestModel(t)
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	result, _ := m.Update(msg)
	updated := result.(Model)
	if updated.width != 120 || updated.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", updated.width, updated.height)
	}
}

// TestUpdate_AgentsLoaded verifies that a loaded agent list is stored on the model.
func TestUpdate_AgentsLoaded(t *testing.T) {
	m := newTestModel(t)
	agents := []*domain.Agent{
		{ID: "1", Name: "test-agent", Model: "sonnet"},
	}
	result, _ := m.Update(agentsLoadedMsg{agents: agents})
	updated := result.(Model)
	if len(updated.agents) != 1 {
		t.Errorf("agents count = %d, want 1", len(updated.agents))
	}
	if updated.agents[0].Name != "test-agent" {
		t.Errorf("agent name = %q, want %q", updated.agents[0].Name, "test-agent")
	}
}

// TestUpdate_SessionsLoaded_ChangesScreen verifies that receiving sessions while
// on screenAgents transitions the model to screenSessionPicker.
func TestUpdate_SessionsLoaded_ChangesScreen(t *testing.T) {
	m := newTestModel(t)
	m.screen = screenAgents
	sessions := []*domain.Session{
		{ID: "s1", AgentID: "a1", Name: "test-session"},
	}
	result, _ := m.Update(sessionsLoadedMsg{sessions: sessions})
	updated := result.(Model)
	if updated.screen != screenSessionPicker {
		t.Errorf("screen = %v, want screenSessionPicker", updated.screen)
	}
}

// TestUpdate_ChatResponse verifies that a chat response clears the waiting flag
// and appends the assistant message.
func TestUpdate_ChatResponse(t *testing.T) {
	m := newTestModel(t)
	m.waiting = true
	result, _ := m.Update(chatResponseMsg{content: "Hello!", cost: 0.01})
	updated := result.(Model)
	if updated.waiting {
		t.Error("should not be waiting after response")
	}
	if len(updated.chatMsgs) != 1 {
		t.Fatalf("chatMsgs count = %d, want 1", len(updated.chatMsgs))
	}
	if updated.chatMsgs[0].Content != "Hello!" {
		t.Errorf("content = %q, want %q", updated.chatMsgs[0].Content, "Hello!")
	}
}

// TestUpdate_ChatError verifies that a chat error clears waiting, sets statusErr,
// and populates the status string.
func TestUpdate_ChatError(t *testing.T) {
	m := newTestModel(t)
	m.waiting = true
	result, _ := m.Update(chatErrorMsg{err: fmt.Errorf("connection failed")})
	updated := result.(Model)
	if updated.waiting {
		t.Error("should not be waiting after error")
	}
	if !updated.statusErr {
		t.Error("statusErr should be true")
	}
	if updated.status == "" {
		t.Error("status should contain error message")
	}
}

// TestUpdate_AgentCreated verifies that a created agent is appended to the list,
// the screen switches to screenAgents, and the cursor points to the new agent.
func TestUpdate_AgentCreated(t *testing.T) {
	m := newTestModel(t)
	m.agents = []*domain.Agent{{ID: "1", Name: "existing"}}
	newAgent := &domain.Agent{ID: "2", Name: "new-agent"}
	result, _ := m.Update(agentCreatedMsg{agent: newAgent})
	updated := result.(Model)
	if len(updated.agents) != 2 {
		t.Errorf("agents count = %d, want 2", len(updated.agents))
	}
	if updated.screen != screenAgents {
		t.Errorf("screen = %v, want screenAgents", updated.screen)
	}
	if updated.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (pointing to new agent)", updated.cursor)
	}
}

// TestUpdate_AgentDeleted verifies that the targeted agent is removed from the
// slice and the status message is set.
func TestUpdate_AgentDeleted(t *testing.T) {
	m := newTestModel(t)
	m.agents = []*domain.Agent{
		{ID: "1", Name: "first"},
		{ID: "2", Name: "second"},
	}
	m.cursor = 1
	result, _ := m.Update(agentDeletedMsg{agentIdx: 0})
	updated := result.(Model)
	if len(updated.agents) != 1 {
		t.Errorf("agents count = %d, want 1", len(updated.agents))
	}
	if updated.status != "agent deleted" {
		t.Errorf("status = %q, want %q", updated.status, "agent deleted")
	}
}

// TestUpdate_SessionCreated verifies that a created session transitions the model
// to screenChat with chatMsgs reset.
func TestUpdate_SessionCreated(t *testing.T) {
	m := newTestModel(t)
	sess := &domain.Session{ID: "s1", AgentID: "a1"}
	result, _ := m.Update(sessionCreatedMsg{session: sess})
	updated := result.(Model)
	if updated.screen != screenChat {
		t.Errorf("screen = %v, want screenChat", updated.screen)
	}
	if updated.currentSess != sess {
		t.Error("currentSess should be the created session")
	}
	if updated.chatMsgs != nil {
		t.Error("chatMsgs should be reset")
	}
}

// TestUpdate_InstallDone_Success verifies that a successful install moves the
// model to screenMenu without setting the error flag.
func TestUpdate_InstallDone_Success(t *testing.T) {
	m := newTestModel(t)
	m.screen = screenInstallConfirm
	result, _ := m.Update(installDoneMsg{installed: true, err: nil})
	updated := result.(Model)
	if updated.screen != screenMenu {
		t.Errorf("screen = %v, want screenMenu", updated.screen)
	}
	if updated.statusErr {
		t.Error("statusErr should be false on success")
	}
}

// TestUpdate_InstallDone_Error verifies that a failed install sets the error flag.
func TestUpdate_InstallDone_Error(t *testing.T) {
	m := newTestModel(t)
	result, _ := m.Update(installDoneMsg{installed: false, err: fmt.Errorf("write failed")})
	updated := result.(Model)
	if !updated.statusErr {
		t.Error("statusErr should be true on error")
	}
}

// TestUpdate_ServiceStatus verifies that the serviceRunning flag and PID are set.
func TestUpdate_ServiceStatus(t *testing.T) {
	m := newTestModel(t)
	result, _ := m.Update(serviceStatusMsg{running: true, pid: "1234"})
	updated := result.(Model)
	if !updated.serviceRunning {
		t.Error("serviceRunning should be true")
	}
	if updated.servicePID != "1234" {
		t.Errorf("servicePID = %q, want %q", updated.servicePID, "1234")
	}
}

// TestView_AllScreens verifies that View() returns a non-empty string for every
// known screen value.
func TestView_AllScreens(t *testing.T) {
	screens := []screen{
		screenMenu, screenAgents, screenNewAgentName,
		screenNewAgentPrompt, screenNewAgentModel, screenNewAgentTools,
		screenSessionPicker, screenChat, screenSessions,
		screenHistory, screenInstallPicker, screenInstallConfirm,
		screenQuickModel,
	}
	// Use a full-length UUID-like ID so sessionDisplayName's [:8] slice is safe.
	const testSessionID = "abcdef12-0000-0000-0000-000000000001"
	for _, s := range screens {
		s := s
		t.Run(fmt.Sprintf("screen_%d", s), func(t *testing.T) {
			m := newTestModel(t)
			m.screen = s
			m.agents = []*domain.Agent{{ID: "1", Name: "test", Model: "sonnet"}}
			m.currentAgent = m.agents[0]
			m.currentSess = &domain.Session{ID: testSessionID, AgentID: "1"}
			view := m.View()
			if view == "" {
				t.Errorf("View() for screen %d returned empty string", s)
			}
		})
	}
}

// TestUpdate_AgentsLoaded_EmptyResetsCursor verifies that loading an empty agent
// list resets the cursor to 0.
func TestUpdate_AgentsLoaded_EmptyResetsCursor(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 5
	result, _ := m.Update(agentsLoadedMsg{agents: []*domain.Agent{}})
	updated := result.(Model)
	if updated.cursor != 0 {
		t.Errorf("cursor = %d, want 0 after empty agents", updated.cursor)
	}
}
