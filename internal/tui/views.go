package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"claudio/internal/domain"
)

func (m Model) renderHeader(title string) string {
	w := m.width
	if w < 40 {
		w = 40
	}
	inner := "claudio v" + domain.Version + "\n" + title
	return headerStyle.Width(min(w-4, 60)).Render(inner) + "\n\n"
}

func (m Model) renderDivider() string {
	w := m.width
	if w < 40 {
		w = 40
	}
	return divStyle.Render(strings.Repeat("─", min(w-2, 60))) + "\n\n"
}

func (m Model) renderFooter(keys string) string {
	s := "\n"
	if m.status != "" {
		if m.statusErr {
			s += errorStyle.Render(m.status)
		} else {
			s += successStyle.Render(m.status)
		}
		s += "\n"
	}
	s += footerStyle.Render(keys)
	return s
}

func (m Model) renderList(items []string, cursor int) string {
	var sb strings.Builder
	for i, item := range items {
		if i == cursor {
			sb.WriteString(cursorStyle.Render("  ▸ "+item) + "\n")
		} else {
			sb.WriteString(normalStyle.Render("    "+item) + "\n")
		}
	}
	return sb.String()
}

func (m Model) renderMenu() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("Claude CLI Proxy & Agent"))

	// Service status banner
	if m.serviceRunning {
		sb.WriteString(successStyle.Render("  ● API running") + dimStyle.Render(fmt.Sprintf("  http://localhost:%s  (PID %s)", m.cfg.Port, m.servicePID)) + "\n\n")
	} else {
		sb.WriteString(dimStyle.Render("  ○ API not running") + "\n\n")
	}

	for i, item := range menuItems {
		label := item.label
		desc := ""
		if item.desc != "" {
			desc = dimStyle.Render("  " + item.desc)
		}
		// Dynamic label for API Service
		if i == 4 {
			if m.serviceRunning {
				label = "Stop API Service"
				desc = dimStyle.Render("  Stop background service")
			} else {
				label = "Start API Service"
				desc = dimStyle.Render("  Run API in background (auto-start at login)")
			}
		}
		if i == m.cursor {
			sb.WriteString(cursorStyle.Render("  ▸ "+label) + desc + "\n")
		} else {
			sb.WriteString(normalStyle.Render("    "+label) + desc + "\n")
		}
	}
	sb.WriteString(m.renderFooter("j/k: navigate  enter: select  q: quit"))
	return sb.String()
}

func (m Model) renderAgents() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader(fmt.Sprintf("Agents (%d)", len(m.agents))))
	if len(m.agents) == 0 {
		sb.WriteString(dimStyle.Render("  No agents yet.") + "\n")
	} else {
		for i, a := range m.agents {
			agentSessions, _ := m.store.ListSessionsByAgent(a.ID)
			sessCount := len(agentSessions)
			line := fmt.Sprintf("%-20s  %-8s  %d sessions",
				truncate(a.Name, 20), truncate(a.Model, 8), sessCount)
			if i == m.cursor {
				sb.WriteString(cursorStyle.Render("  ▸ "+line) + "\n")
			} else {
				sb.WriteString(normalStyle.Render("    "+line) + "\n")
			}
		}
	}
	sb.WriteString(m.renderFooter("n: new agent  enter: chat with agent  d: delete  esc: back"))
	return sb.String()
}

func (m Model) renderNewAgentName() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("New Agent"))
	sb.WriteString(normalStyle.Render("  Name:") + "\n")
	sb.WriteString("  > " + m.input.View() + "\n")
	sb.WriteString(m.renderFooter("enter: next  esc: cancel"))
	return sb.String()
}

func (m Model) renderNewAgentPrompt() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("New Agent"))
	sb.WriteString(dimStyle.Render("  Name: "+m.newAgentName) + "\n\n")
	sb.WriteString(normalStyle.Render("  System Prompt:") + "\n")
	sb.WriteString("  > " + m.input.View() + "\n")
	sb.WriteString(m.renderFooter("enter: next  esc: cancel"))
	return sb.String()
}

func (m Model) renderNewAgentModel() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("New Agent"))
	sb.WriteString(dimStyle.Render("  Name: "+m.newAgentName) + "\n")
	sb.WriteString(dimStyle.Render("  System Prompt: "+truncate(m.newAgentPrompt, 40)) + "\n\n")
	sb.WriteString(normalStyle.Render("  Model:") + "\n")
	sb.WriteString(m.renderList(m.items, m.cursor))
	sb.WriteString(m.renderFooter("j/k: navigate  enter: next  esc: cancel"))
	return sb.String()
}

func (m Model) renderNewAgentTools() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("New Agent"))
	sb.WriteString(dimStyle.Render("  Name: "+m.newAgentName) + "\n")
	sb.WriteString(dimStyle.Render("  System Prompt: "+truncate(m.newAgentPrompt, 40)) + "\n")
	sb.WriteString(dimStyle.Render("  Model: "+m.newAgentModel) + "\n\n")
	sb.WriteString(normalStyle.Render("  Allowed Tools:") + "\n\n")
	allSel := m.allToolsSelected()
	check := "[ ]"
	if allSel {
		check = "[✓]"
	}
	line := fmt.Sprintf("  %s Select All", check)
	if m.toolsCursor == 0 {
		sb.WriteString(cursorStyle.Render("▸ "+line[2:]) + "\n")
	} else {
		sb.WriteString(dimStyle.Render(line) + "\n")
	}
	sb.WriteString(dimStyle.Render("  "+strings.Repeat("─", 30)) + "\n")
	for i, tool := range availableTools {
		check := "[ ]"
		if m.toolsSelected[i] {
			check = "[✓]"
		}
		line := fmt.Sprintf("  %s %s", check, tool)
		if m.toolsCursor == i+1 {
			sb.WriteString(cursorStyle.Render("▸ "+line[2:]) + "\n")
		} else {
			sb.WriteString(normalStyle.Render(line) + "\n")
		}
	}
	sb.WriteString(m.renderFooter("space: toggle  enter: create  esc: back"))
	return sb.String()
}

func (m Model) renderSessionPicker() string {
	var sb strings.Builder
	agentName := ""
	if m.currentAgent != nil {
		agentName = m.currentAgent.Name
	}
	sb.WriteString(m.renderHeader(agentName + " — Sessions"))
	totalItems := 1 + len(m.sessions)
	items := make([]string, totalItems)
	items[0] = "New session"
	for i, s := range m.sessions {
		items[i+1] = fmt.Sprintf("%-25s  %d turns", truncate(sessionDisplayName(s), 25), s.TurnCount)
	}
	sb.WriteString(m.renderList(items, m.cursor))
	sb.WriteString(m.renderFooter("enter: select  esc: back"))
	return sb.String()
}

func (m Model) renderChatScreen() string {
	var sb strings.Builder
	title := "Quick Chat"
	if m.currentAgent != nil {
		title = m.currentAgent.Name + " (" + m.currentAgent.Model + ")"
		if m.currentSess != nil && !m.quickChat {
			title = m.currentAgent.Name + " > " + sessionDisplayName(m.currentSess) + " (" + m.currentAgent.Model + ")"
		}
	}
	sb.WriteString(m.renderHeader(title))
	maxMsgs := m.height - 12
	if maxMsgs < 2 {
		maxMsgs = 2
	}
	msgs := m.chatMsgs
	if len(msgs) > maxMsgs {
		msgs = msgs[len(msgs)-maxMsgs:]
	}
	for _, msg := range msgs {
		if msg.Role == "user" {
			sb.WriteString(userMsgStyle.Render("[you]") + " " + msg.Content + "\n")
		} else {
			sb.WriteString(aiMsgStyle.Render("[claude]") + " " + msg.Content + "\n")
		}
	}
	sb.WriteString("\n")
	if m.sessionCost > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  session cost: $%.6f", m.sessionCost)) + "\n")
	}
	if m.waiting {
		sb.WriteString("  " + m.spinner.View() + " Waiting for response...\n")
	} else {
		sb.WriteString("  > " + m.input.View() + "\n")
	}
	sb.WriteString(m.renderFooter("enter: send  esc: back to menu"))
	return sb.String()
}

func (m Model) renderSessionsScreen() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader(fmt.Sprintf("Sessions (%d)", len(m.allSessions))))
	if len(m.allSessions) == 0 {
		sb.WriteString(dimStyle.Render("  No sessions yet.") + "\n")
	} else {
		sb.WriteString(m.renderList(m.items, m.cursor))
	}
	sb.WriteString(m.renderFooter("enter: view history  d: delete  esc: back"))
	return sb.String()
}

func (m Model) renderHistory() string {
	var sb strings.Builder
	title := "History"
	if len(m.allSessions) > 0 && m.cursor < len(m.allSessions) {
		sa := m.allSessions[m.cursor]
		title = sa.agentName + " / " + sessionDisplayName(sa.sess) +
			fmt.Sprintf(" — History (%d messages)", len(m.messages))
	}
	sb.WriteString(m.renderHeader(title))
	sb.WriteString(m.viewport.View())
	sb.WriteString(m.renderFooter("↑/↓: scroll  esc: back"))
	return sb.String()
}

func (m Model) renderInstallPicker() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("Install Agent to Claude Code"))
	sb.WriteString(normalStyle.Render("  Select an agent to install:") + "\n\n")
	for i, a := range m.agents {
		line := fmt.Sprintf("%-20s  %-8s", truncate(a.Name, 20), truncate(a.Model, 8))
		if i == m.cursor {
			sb.WriteString(cursorStyle.Render("  ▸ "+line) + "\n")
		} else {
			sb.WriteString(normalStyle.Render("    "+line) + "\n")
		}
	}
	sb.WriteString(m.renderFooter("enter: install  u: uninstall  esc: back"))
	return sb.String()
}

func (m Model) renderQuickModel() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader("Quick Chat — Select Model"))
	sb.WriteString(m.renderList(m.items, m.cursor))
	sb.WriteString(m.renderFooter("j/k: navigate  enter: select  esc: back"))
	return sb.String()
}

func (m Model) renderInstallConfirm() string {
	var sb strings.Builder
	agentName := ""
	if m.currentAgent != nil {
		agentName = m.currentAgent.Name
	}
	sb.WriteString(m.renderHeader(fmt.Sprintf("Install %q?", agentName)))
	target := filepath.Join(m.cfg.WorkDir, ".claude", "agents", domain.SanitizeName(agentName)+".md")
	sb.WriteString(normalStyle.Render(fmt.Sprintf("  Target: %s", target)) + "\n\n")
	sb.WriteString(m.renderFooter("y: install  u: uninstall  n/esc: cancel"))
	return sb.String()
}
