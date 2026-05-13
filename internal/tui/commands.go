// Package tui provides the Bubble Tea terminal interface for claudio.
package tui

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/service"
	"claudio/internal/store"
)

func loadAgentsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		all, err := st.ListAgents()
		if err != nil {
			return chatErrorMsg{err: err}
		}
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
		sessions, err := st.ListSessionsByAgent(agentID)
		if err != nil {
			return chatErrorMsg{err: err}
		}
		return sessionsLoadedMsg{sessions: sessions}
	}
}

func loadMessagesCmd(workDir, sessionID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := store.ReadSessionMessages(workDir, sessionID)
		if err != nil {
			return chatErrorMsg{err: err}
		}
		return messagesLoadedMsg{messages: msgs}
	}
}

func loadAllSessionsCmd(st *store.Store, agents []*domain.Agent) tea.Cmd {
	return func() tea.Msg {
		var all []sessionWithAgent
		for _, a := range agents {
			sessions, _ := st.ListSessionsByAgent(a.ID)
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
		var inputTokens, outputTokens, cacheCreation, cacheRead int
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
					Usage  struct {
						InputTokens              int `json:"input_tokens"`
						OutputTokens             int `json:"output_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					} `json:"usage"`
				}
				json.Unmarshal(data, &r) //nolint:errcheck — best-effort cost extraction from optional field
				cost = r.Cost
				inputTokens = r.Usage.InputTokens
				outputTokens = r.Usage.OutputTokens
				cacheCreation = r.Usage.CacheCreationInputTokens
				cacheRead = r.Usage.CacheReadInputTokens
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
			st.SaveSession(sess) //nolint:errcheck — non-critical; session state is ephemeral
			// Persist token usage — non-fatal on error (P3, P6, P7).
			tu := domain.TokenUsage{
				SessionID:                sess.ID,
				AgentID:                  agent.ID,
				InputTokens:              inputTokens,
				OutputTokens:             outputTokens,
				CacheCreationInputTokens: cacheCreation,
				CacheReadInputTokens:     cacheRead,
				TotalCostUSD:             cost,
			}
			if saveErr := st.SaveTokenUsage(tu); saveErr != nil {
				log.Printf("tui: failed to save token usage: %v", saveErr)
			}
		}
		return chatResponseMsg{
			content:       text,
			cost:          cost,
			inputTokens:   inputTokens,
			outputTokens:  outputTokens,
			cacheCreation: cacheCreation,
			cacheRead:     cacheRead,
		}
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
