package tui

import (
	"fmt"
	"strings"
	"time"

	"claudio/internal/domain"
)

func sessionDisplayName(s *domain.Session) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID[:8]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func relativeTime(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return "?"
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func renderMsgHistory(msgs []domain.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		if m.Role == "user" {
			sb.WriteString(userMsgStyle.Render("[you]") + " " + m.Content + "\n\n")
		} else {
			sb.WriteString(aiMsgStyle.Render("[claude]") + " " + m.Content + "\n\n")
		}
	}
	if sb.Len() == 0 {
		return dimStyle.Render("  No messages yet.")
	}
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
