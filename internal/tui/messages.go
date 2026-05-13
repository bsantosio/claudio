package tui

import "claudio/internal/domain"

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
