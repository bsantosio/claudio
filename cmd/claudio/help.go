package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"claudio/internal/domain"
)

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func cmdPrompt() {
	fmt.Print(domain.AIPrompt)
	if err := copyToClipboard(domain.AIPrompt); err == nil {
		fmt.Println("\n✓ Copied to clipboard")
	}
}

func printUsage() {
	fmt.Print(`claudio — Claude CLI proxy with agent management

Usage:
  claudio                    Interactive TUI (default)
  claudio web                Start web UI + open browser
  claudio mcp                Start MCP server over stdio
  claudio prompt             Print AI-ready usage prompt (+ copy to clipboard)
  claudio service install    Install background service (auto-start at login)
  claudio service uninstall  Remove background service
  claudio service status     Check service status
  claudio version            Print version

Environment:
  PORT               HTTP port (default: 18080)
  ADAPTER_API_KEY    Bearer token for API auth (optional)
  DEFAULT_MODEL      Default model: sonnet, haiku, opus (default: sonnet)
  WORK_DIR           Working directory for Claude CLI (default: .)
  DATA_DIR           SQLite database location (default: ./data)

Install:
  brew install bsantosio/tap/claudio
`)
}
