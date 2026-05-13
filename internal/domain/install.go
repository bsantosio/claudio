package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reSanitize = regexp.MustCompile(`[^a-z0-9]+`)

func SanitizeName(name string) string {
	lower := strings.ToLower(name)
	replaced := reSanitize.ReplaceAllString(lower, "-")
	return strings.Trim(replaced, "-")
}

func InstallAgent(agent *Agent, workDir string) error {
	dir := filepath.Join(workDir, ".claude", "agents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	var buf strings.Builder
	buf.WriteString("---\n")
	buf.WriteString("model: " + agent.Model + "\n")
	if len(agent.AllowedTools) > 0 {
		buf.WriteString("allowedTools:\n")
		for _, t := range agent.AllowedTools {
			buf.WriteString("  - " + t + "\n")
		}
	}
	buf.WriteString("---\n\n")
	buf.WriteString(agent.SystemPrompt + "\n")
	filename := SanitizeName(agent.Name) + ".md"
	path := filepath.Join(dir, filename)
	return os.WriteFile(path, []byte(buf.String()), 0644)
}

func UninstallAgent(agent *Agent, workDir string) error {
	filename := SanitizeName(agent.Name) + ".md"
	path := filepath.Join(workDir, ".claude", "agents", filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove agent file: %w", err)
	}
	return nil
}
