package tui

import "github.com/charmbracelet/lipgloss"

var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 2).Align(lipgloss.Center)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	normalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	userMsgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	aiMsgStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	divStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
)

var availableTools = []string{
	"Bash", "Read", "Edit", "Write", "Glob", "Grep",
	"WebSearch", "WebFetch", "Agent", "NotebookEdit",
}
