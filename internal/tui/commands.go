package tui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"claudio/internal/service"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
)

func loadAgentsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		all := st.ListAgents()
		var filtered []*domain.Agent
		for _, a := range all {
			if a.Name != "__quick_chat__" {
				filtered = append(filtered, a)
			}
		}
		return agentsLoadedMsg{agents: filtered}
	}
}

func loadSessionsCmd(st *store.Store, agentID string) tea.Cmd {
	return func() tea.Msg {
		return sessionsLoadedMsg{sessions: st.ListSessionsByAgent(agentID)}
	}
}

func loadMessagesCmd(workDir, sessionID string) tea.Cmd {
	return func() tea.Msg {
		msgs, _ := store.ReadSessionMessages(workDir, sessionID)
		return messagesLoadedMsg{messages: msgs}
	}
}

func loadAllSessionsCmd(st *store.Store, agents []*domain.Agent) tea.Cmd {
	return func() tea.Msg {
		var all []sessionWithAgent
		for _, a := range agents {
			sessions := st.ListSessionsByAgent(a.ID)
			for _, s := range sessions {
				all = append(all, sessionWithAgent{sess: s, agentName: a.Name})
			}
		}
		return allSessionsLoadedMsg{sessions: all}
	}
}

func sendMessageCmd(cfg domain.Config, st *store.Store, agent *domain.Agent, sess *domain.Session, content string, ephemeral bool) tea.Cmd {
	return func() tea.Msg {
		var response strings.Builder
		var cost float64
		resume := sess.TurnCount > 0
		sess.TurnCount++
		ctx := context.Background()
		err := claude.RunClaude(ctx, cfg, agent, sess.ID, content, resume, func(eventType string, data []byte) error {
			switch eventType {
			case "assistant":
				ev, parseErr := claude.ParseNDJSON(string(data))
				if parseErr != nil {
					return nil
				}
				text, _ := claude.ExtractText(ev)
				response.WriteString(text)
			case "result":
				var r struct {
					Result string  `json:"result"`
					Cost   float64 `json:"total_cost_usd"`
				}
				_ = json.Unmarshal(data, &r)
				cost = r.Cost
				if response.Len() == 0 && r.Result != "" {
					response.WriteString(r.Result)
				}
			}
			return nil
		})
		if err != nil {
			return chatErrorMsg{err: err}
		}
		text := strings.TrimSpace(response.String())
		if text == "" {
			text = "(no response)"
		}
		if !ephemeral {
			sess.LastActive = time.Now().UTC().Format(time.RFC3339Nano)
			_ = st.SaveSession(sess)
		}
		return chatResponseMsg{content: text, cost: cost}
	}
}

func createAgentCmd(st *store.Store, defaultModel, name, prompt, model, tools string) tea.Cmd {
	return func() tea.Msg {
		if model == "" {
			model = defaultModel
		}
		var allowedTools []string
		if tools != "" {
			for _, t := range strings.Split(tools, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					allowedTools = append(allowedTools, t)
				}
			}
		}
		input := domain.Agent{Name: name, SystemPrompt: prompt, Model: model, AllowedTools: allowedTools}
		agent, err := st.CreateAgent(input, defaultModel)
		if err != nil {
			return chatErrorMsg{err: err}
		}
		return agentCreatedMsg{agent: agent}
	}
}

func deleteAgentCmd(st *store.Store, agent *domain.Agent, idx int) tea.Cmd {
	return func() tea.Msg {
		if err := st.DeleteAgent(agent.ID); err != nil {
			return chatErrorMsg{err: err}
		}
		return agentDeletedMsg{agentIdx: idx}
	}
}

func createSessionCmd(st *store.Store, agentID, name string) tea.Cmd {
	return func() tea.Msg {
		sess, err := st.CreateSession(agentID, name)
		if err != nil {
			return chatErrorMsg{err: err}
		}
		return sessionCreatedMsg{session: sess}
	}
}

func deleteSessionCmd(st *store.Store, sessionID string, idx int) tea.Cmd {
	return func() tea.Msg {
		if err := st.DeleteSession(sessionID); err != nil {
			return chatErrorMsg{err: err}
		}
		return sessionDeletedMsg{sessIdx: idx}
	}
}

func installAgentCmd(agent *domain.Agent, workDir string) tea.Cmd {
	return func() tea.Msg {
		err := domain.InstallAgent(agent, workDir)
		return installDoneMsg{installed: true, err: err}
	}
}

func uninstallAgentCmd(agent *domain.Agent, workDir string) tea.Cmd {
	return func() tea.Msg {
		err := domain.UninstallAgent(agent, workDir)
		return installDoneMsg{installed: false, err: err}
	}
}

func checkServiceCmd() tea.Cmd {
	return func() tea.Msg {
		running, pid, _ := service.Status()
		return serviceStatusMsg{running: running, pid: pid}
	}
}

func installServiceCmd(port string) tea.Cmd {
	return func() tea.Msg {
		err := service.Install(port)
		if err == nil {
			time.Sleep(2 * time.Second)
		}
		return serviceActionMsg{action: "installed", err: err}
	}
}

func uninstallServiceCmd() tea.Cmd {
	return func() tea.Msg {
		err := service.Uninstall()
		if err == nil {
			time.Sleep(500 * time.Millisecond)
		}
		return serviceActionMsg{action: "removed", err: err}
	}
}
