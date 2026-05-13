package tui

import (
	"fmt"

	"claudio/internal/domain"
	"claudio/internal/store"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenMenu screen = iota
	screenAgents
	screenNewAgentName
	screenNewAgentPrompt
	screenNewAgentModel
	screenNewAgentTools
	screenSessionPicker
	screenChat
	screenSessions
	screenHistory
	screenInstallPicker
	screenInstallConfirm
)

type chatResponseMsg struct {
	content string
	cost    float64
}

type chatErrorMsg struct {
	err error
}

type agentsLoadedMsg struct {
	agents []*domain.Agent
}

type sessionsLoadedMsg struct {
	sessions []*domain.Session
}

type messagesLoadedMsg struct {
	messages []domain.Message
}

type agentCreatedMsg struct {
	agent *domain.Agent
}

type agentDeletedMsg struct {
	agentIdx int
}

type sessionCreatedMsg struct {
	session *domain.Session
}

type sessionDeletedMsg struct {
	sessIdx int
}

type installDoneMsg struct {
	installed bool
	err       error
}

type serviceStatusMsg struct {
	running bool
	pid     string
}

type serviceActionMsg struct {
	action string
	err    error
}

type sessionWithAgent struct {
	sess      *domain.Session
	agentName string
}

type allSessionsLoadedMsg struct {
	sessions []sessionWithAgent
}

type Model struct {
	cfg   domain.Config
	store *store.Store

	screen screen

	items  []string
	cursor int

	input textinput.Model

	chatMsgs []domain.Message
	waiting  bool
	spinner  spinner.Model

	agents       []*domain.Agent
	sessions     []*domain.Session
	allSessions  []sessionWithAgent
	currentAgent *domain.Agent
	currentSess  *domain.Session
	messages     []domain.Message

	newAgentName   string
	newAgentPrompt string
	newAgentModel  string
	toolsCursor    int
	toolsSelected  map[int]bool

	quickChat bool

	serviceRunning bool
	servicePID     string

	viewport viewport.Model

	width, height int

	status    string
	statusErr bool
}

func NewModel(cfg domain.Config, st *store.Store) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	ti := textinput.New()
	ti.CharLimit = 0
	vp := viewport.New(80, 20)
	return Model{
		cfg:      cfg,
		store:    st,
		screen:   screenMenu,
		spinner:  sp,
		input:    ti,
		viewport: vp,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadAgentsCmd(m.store), checkServiceCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 8
		return m, nil
	case agentsLoadedMsg:
		m.agents = msg.agents
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
		return m, nil
	case sessionsLoadedMsg:
		m.sessions = msg.sessions
		if m.screen == screenAgents {
			m.screen = screenSessionPicker
			m.cursor = 0
			m.status = ""
		}
		return m, nil
	case allSessionsLoadedMsg:
		m.allSessions = msg.sessions
		m.items = make([]string, len(m.allSessions))
		for i, sa := range m.allSessions {
			turns := sa.sess.TurnCount
			ago := relativeTime(sa.sess.LastActive)
			m.items[i] = fmt.Sprintf("%-20s / %-20s  %d turns   %s",
				truncate(sa.agentName, 20),
				truncate(sessionDisplayName(sa.sess), 20),
				turns, ago)
		}
		m.cursor = 0
		return m, nil
	case messagesLoadedMsg:
		m.messages = msg.messages
		content := renderMsgHistory(msg.messages)
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()
		if m.screen == screenSessions {
			m.screen = screenHistory
			m.status = ""
		}
		return m, nil
	case chatResponseMsg:
		m.waiting = false
		m.chatMsgs = append(m.chatMsgs, domain.Message{Role: "assistant", Content: msg.content})
		return m, nil
	case chatErrorMsg:
		m.waiting = false
		m.status = "error: " + msg.err.Error()
		m.statusErr = true
		return m, nil
	case agentCreatedMsg:
		m.agents = append(m.agents, msg.agent)
		m.screen = screenAgents
		m.cursor = len(m.agents) - 1
		m.status = "agent \"" + msg.agent.Name + "\" created"
		m.statusErr = false
		return m, nil
	case agentDeletedMsg:
		m.agents = append(m.agents[:msg.agentIdx], m.agents[msg.agentIdx+1:]...)
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
		m.status = "agent deleted"
		m.statusErr = false
		return m, nil
	case sessionCreatedMsg:
		m.currentSess = msg.session
		m.chatMsgs = nil
		m.screen = screenChat
		m.input.Placeholder = ""
		m.input.SetValue("")
		m.input.Focus()
		return m, nil
	case sessionDeletedMsg:
		if m.screen == screenSessions {
			m.allSessions = append(m.allSessions[:msg.sessIdx], m.allSessions[msg.sessIdx+1:]...)
			m.items = append(m.items[:msg.sessIdx], m.items[msg.sessIdx+1:]...)
			if m.cursor >= len(m.items) {
				m.cursor = max(0, len(m.items)-1)
			}
		} else {
			m.sessions = append(m.sessions[:msg.sessIdx], m.sessions[msg.sessIdx+1:]...)
			if m.cursor > 0 {
				m.cursor--
			}
		}
		m.status = "session deleted"
		m.statusErr = false
		return m, nil
	case serviceStatusMsg:
		m.serviceRunning = msg.running
		m.servicePID = msg.pid
		return m, nil
	case serviceActionMsg:
		if msg.err != nil {
			m.status = "service error: " + msg.err.Error()
			m.statusErr = true
		} else {
			m.status = "service " + msg.action
			m.statusErr = false
		}
		return m, checkServiceCmd()
	case installDoneMsg:
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			m.statusErr = true
		} else {
			if msg.installed {
				m.status = "agent installed to .claude/agents/"
			} else {
				m.status = "agent uninstalled"
			}
			m.statusErr = false
		}
		m.screen = screenMenu
		return m, nil
	case spinner.TickMsg:
		if m.waiting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(msg)
	case screenAgents:
		return m.updateAgents(msg)
	case screenNewAgentName:
		return m.updateNewAgentName(msg)
	case screenNewAgentPrompt:
		return m.updateNewAgentPrompt(msg)
	case screenNewAgentModel:
		return m.updateNewAgentModel(msg)
	case screenNewAgentTools:
		return m.updateNewAgentTools(msg)
	case screenSessionPicker:
		return m.updateSessionPicker(msg)
	case screenChat:
		return m.updateChat(msg)
	case screenSessions:
		return m.updateSessions(msg)
	case screenHistory:
		return m.updateHistory(msg)
	case screenInstallPicker:
		return m.updateInstallPicker(msg)
	case screenInstallConfirm:
		return m.updateInstallConfirm(msg)
	}
	return m, nil
}

func (m Model) View() string {
	switch m.screen {
	case screenMenu:
		return m.renderMenu()
	case screenAgents:
		return m.renderAgents()
	case screenNewAgentName:
		return m.renderNewAgentName()
	case screenNewAgentPrompt:
		return m.renderNewAgentPrompt()
	case screenNewAgentModel:
		return m.renderNewAgentModel()
	case screenNewAgentTools:
		return m.renderNewAgentTools()
	case screenSessionPicker:
		return m.renderSessionPicker()
	case screenChat:
		return m.renderChatScreen()
	case screenSessions:
		return m.renderSessionsScreen()
	case screenHistory:
		return m.renderHistory()
	case screenInstallPicker:
		return m.renderInstallPicker()
	case screenInstallConfirm:
		return m.renderInstallConfirm()
	}
	return ""
}
