package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claudio/internal/domain"
	"claudio/internal/store"
)

var menuItems = []struct {
	label string
	desc  string
}{
	{"Quick Chat", "Chat directly with Claude"},
	{"Agents", "Manage specialized agents"},
	{"Sessions", "View and manage sessions"},
	{"Install Agent", "Export agent to Claude Code"},
	{"API Service", ""},
	{"Quit", ""},
}

func (m Model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0:
				return m.startQuickChat()
			case 1:
				m.screen = screenAgents
				m.cursor = 0
				m.status = ""
				return m, loadAgentsCmd(m.store)
			case 2:
				m.screen = screenSessions
				m.cursor = 0
				m.status = ""
				return m, loadAllSessionsCmd(m.store, m.agents)
			case 3:
				if len(m.agents) == 0 {
					m.status = "No agents yet. Create one first."
					m.statusErr = false
					return m, nil
				}
				m.screen = screenInstallPicker
				m.cursor = 0
				m.status = ""
				return m, nil
			case 4:
				if m.serviceRunning {
					return m, uninstallServiceCmd()
				}
				return m, installServiceCmd(m.cfg.Port)
			case 5:
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) startQuickChat() (Model, tea.Cmd) {
	m.screen = screenQuickModel
	m.items = []string{"sonnet", "opus", "haiku", "claude-opus-4-6[1m]"}
	m.cursor = 0
	m.status = ""
	return m, nil
}

func (m Model) updateQuickModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenMenu
			m.cursor = 0
			m.status = ""
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			model := m.items[m.cursor]
			sessionID, _ := domain.NewUUID()
			m.currentAgent = &domain.Agent{
				Name:         "Quick Chat",
				SystemPrompt: "You are a helpful assistant.",
				Model:        model,
			}
			m.currentSess = &domain.Session{
				ID:   sessionID,
				Name: "quick",
			}
			m.chatMsgs = nil
			m.screen = screenChat
			m.input.Focus()
			m.status = ""
			m.quickChat = true
		}
	}
	return m, nil
}

func (m Model) updateAgents(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenMenu
			m.cursor = 0
			m.status = ""
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.agents) > 0 && m.cursor < len(m.agents) {
				agent := m.agents[m.cursor]
				m.currentAgent = agent
				m.status = ""
				return m, loadSessionsCmd(m.store, agent.ID)
			}
		case "n":
			m.screen = screenNewAgentName
			m.newAgentName = ""
			m.newAgentPrompt = ""
			m.newAgentModel = ""
			m.input.Placeholder = ""
			m.input.SetValue("")
			m.input.Focus()
			m.status = ""
		case "d":
			if len(m.agents) > 0 && m.cursor < len(m.agents) {
				agent := m.agents[m.cursor]
				return m, deleteAgentCmd(m.store, agent, m.cursor)
			}
		}
	}
	return m, nil
}

func (m Model) updateNewAgentName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenAgents
			m.cursor = 0
			m.status = ""
			return m, nil
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				m.status = "name is required"
				m.statusErr = true
				return m, nil
			}
			m.newAgentName = name
			m.screen = screenNewAgentPrompt
			m.input.Placeholder = ""
			m.input.SetValue("")
			m.input.Focus()
			m.status = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateNewAgentPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenNewAgentName
			m.input.SetValue(m.newAgentName)
			m.input.Focus()
			m.status = ""
			return m, nil
		case "enter":
			prompt := strings.TrimSpace(m.input.Value())
			if prompt == "" {
				m.status = "system prompt is required"
				m.statusErr = true
				return m, nil
			}
			m.newAgentPrompt = prompt
			m.screen = screenNewAgentModel
			m.items = []string{"sonnet", "opus", "haiku", "claude-opus-4-6[1m]"}
			m.cursor = 0
			m.status = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateNewAgentModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenNewAgentPrompt
			m.input.SetValue(m.newAgentPrompt)
			m.input.Focus()
			m.status = ""
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.newAgentModel = m.items[m.cursor]
			m.screen = screenNewAgentTools
			m.toolsCursor = 0
			m.toolsSelected = make(map[int]bool)
			m.status = ""
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateNewAgentTools(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalItems := 1 + len(availableTools)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenNewAgentModel
			m.cursor = 0
			for i, item := range m.items {
				if item == m.newAgentModel {
					m.cursor = i
					break
				}
			}
			m.status = ""
			return m, nil
		case "up", "k":
			if m.toolsCursor > 0 {
				m.toolsCursor--
			}
		case "down", "j":
			if m.toolsCursor < totalItems-1 {
				m.toolsCursor++
			}
		case " ":
			if m.toolsCursor == 0 {
				allSelected := m.allToolsSelected()
				for i := range availableTools {
					if allSelected {
						delete(m.toolsSelected, i)
					} else {
						m.toolsSelected[i] = true
					}
				}
			} else {
				idx := m.toolsCursor - 1
				if m.toolsSelected[idx] {
					delete(m.toolsSelected, idx)
				} else {
					m.toolsSelected[idx] = true
				}
			}
		case "enter":
			var selected []string
			for i, t := range availableTools {
				if m.toolsSelected[i] {
					selected = append(selected, t)
				}
			}
			tools := strings.Join(selected, ",")
			return m, createAgentCmd(m.store, m.cfg.DefaultModel, m.newAgentName, m.newAgentPrompt, m.newAgentModel, tools)
		}
	}
	return m, nil
}

func (m Model) allToolsSelected() bool {
	if len(m.toolsSelected) == 0 {
		return false
	}
	for i := range availableTools {
		if !m.toolsSelected[i] {
			return false
		}
	}
	return true
}

func (m Model) updateSessionPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalItems := 1 + len(m.sessions)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenAgents
			m.cursor = 0
			m.status = ""
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < totalItems-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor == 0 {
				name := "session-" + time.Now().Format("20060102-150405")
				return m, createSessionCmd(m.store, m.currentAgent.ID, name)
			}
			sessIdx := m.cursor - 1
			if sessIdx < len(m.sessions) {
				sess := m.sessions[sessIdx]
				m.currentSess = sess
				msgs, _ := store.ReadSessionMessages(m.cfg.WorkDir, sess.ID)
				m.chatMsgs = msgs
				m.screen = screenChat
				m.input.Placeholder = ""
				m.input.SetValue("")
				m.input.Focus()
				m.status = ""
			}
		}
	}
	return m, nil
}

func (m Model) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if !m.waiting {
				if m.quickChat {
					m.quickChat = false
					m.screen = screenMenu
					m.cursor = 0
				} else {
					m.screen = screenAgents
					m.cursor = 0
				}
				m.status = ""
			}
			return m, nil
		case "enter":
			if m.waiting {
				return m, nil
			}
			content := strings.TrimSpace(m.input.Value())
			if content == "" {
				return m, nil
			}
			m.input.SetValue("")
			m.chatMsgs = append(m.chatMsgs, domain.Message{Role: "user", Content: content})
			m.waiting = true
			if m.currentAgent == nil || m.currentSess == nil {
				return m, nil
			}
			return m, tea.Batch(
				sendMessageCmd(m.cfg, m.store, m.currentAgent, m.currentSess, content, m.quickChat),
				m.spinner.Tick,
			)
		}
	}
	if !m.waiting {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateSessions(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenMenu
			m.cursor = 0
			m.status = ""
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.allSessions)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.allSessions) > 0 && m.cursor < len(m.allSessions) {
				sa := m.allSessions[m.cursor]
				return m, loadMessagesCmd(m.cfg.WorkDir, sa.sess.ID)
			}
		case "d":
			if len(m.allSessions) > 0 && m.cursor < len(m.allSessions) {
				sa := m.allSessions[m.cursor]
				return m, deleteSessionCmd(m.store, sa.sess.ID, m.cursor)
			}
		}
	}
	return m, nil
}

func (m Model) updateHistory(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenSessions
			m.status = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) updateInstallPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.screen = screenMenu
			m.cursor = 0
			m.status = ""
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.agents) > 0 && m.cursor < len(m.agents) {
				m.currentAgent = m.agents[m.cursor]
				m.screen = screenInstallConfirm
				m.status = ""
			}
		}
	}
	return m, nil
}

func (m Model) updateInstallConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "n":
			m.screen = screenInstallPicker
			m.status = ""
		case "y":
			if m.currentAgent != nil {
				return m, installAgentCmd(m.currentAgent, m.cfg.WorkDir)
			}
		case "u":
			if m.currentAgent != nil {
				return m, uninstallAgentCmd(m.currentAgent, m.cfg.WorkDir)
			}
		}
	}
	return m, nil
}
