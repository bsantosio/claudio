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
	buf.WriteString("name: " + SanitizeName(agent.Name) + "\n")
	buf.WriteString("description: >-\n")
	// Use first line of system prompt as description, truncated
	desc := agent.SystemPrompt
	if idx := strings.Index(desc, "\n"); idx > 0 {
		desc = desc[:idx]
	}
	if len(desc) > 200 {
		desc = desc[:200]
	}
	buf.WriteString("  " + strings.TrimSpace(desc) + "\n")
	buf.WriteString("model: " + agent.Model + "\n")

	// Build tools list — comma-separated string (correct Claude Code format)
	tools := buildInstallTools(agent)
	if len(tools) > 0 {
		buf.WriteString("tools: " + strings.Join(tools, ", ") + "\n")
	}

	buf.WriteString("---\n\n")

	// System prompt body
	buf.WriteString(agent.SystemPrompt + "\n")

	// If sub-agents are defined, add context
	if len(agent.SubAgents) > 0 {
		buf.WriteString("\n## Available Sub-Agents\n\n")
		buf.WriteString("You can delegate work to these sub-agents using the Agent tool:\n\n")
		for _, sa := range agent.SubAgents {
			buf.WriteString("- **" + sa + "**\n")
		}
		buf.WriteString("\nUse `Agent({ subagent_type: \"<name>\", prompt: \"...\" })` to invoke them.\n")
	}

	filename := SanitizeName(agent.Name) + ".md"
	path := filepath.Join(dir, filename)
	return os.WriteFile(path, []byte(buf.String()), 0644)
}

func buildInstallTools(agent *Agent) []string {
	tools := make([]string, len(agent.AllowedTools))
	copy(tools, agent.AllowedTools)

	// If sub-agents defined, ensure Agent tool is included
	if len(agent.SubAgents) > 0 {
		hasAgent := false
		for _, t := range tools {
			if t == "Agent" {
				hasAgent = true
				break
			}
		}
		if !hasAgent {
			tools = append(tools, "Agent")
		}
	}

	return tools
}

func UninstallAgent(agent *Agent, workDir string) error {
	filename := SanitizeName(agent.Name) + ".md"
	path := filepath.Join(workDir, ".claude", "agents", filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove agent file: %w", err)
	}
	return nil
}
