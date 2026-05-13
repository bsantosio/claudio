package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"claudio/internal/domain"
)

type StreamCallback func(eventType string, data []byte) error

type StreamEvent struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	RawLine   string          `json:"-"`
}

type Runner func(ctx context.Context, cfg domain.Config, agent *domain.Agent, sessionID string, message string, resume bool, onEvent StreamCallback) error

func ParseNDJSON(line string) (StreamEvent, error) {
	var ev StreamEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return StreamEvent{}, fmt.Errorf("ndjson parse error: %w", err)
	}
	ev.RawLine = line
	return ev, nil
}

func ExtractText(ev StreamEvent) (string, error) {
	if ev.Message == nil {
		return "", nil
	}
	var msg struct {
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		} `json:"content"`
	}
	if err := json.Unmarshal(ev.Message, &msg); err != nil {
		return "", fmt.Errorf("extract text: %w", err)
	}
	var parts []string
	for _, c := range msg.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
		if c.Delta.Type == "text_delta" && c.Delta.Text != "" {
			parts = append(parts, c.Delta.Text)
		}
	}
	return strings.Join(parts, ""), nil
}

func BuildClaudeArgs(agent *domain.Agent, sessionID, message string, resume bool) []string {
	args := []string{
		"-p",
		"--verbose",
		"--output-format", "stream-json",
		"--model", agent.Model,
	}
	if resume {
		args = append(args, "--resume", sessionID)
	} else {
		args = append(args, "--session-id", sessionID, "--system-prompt", agent.SystemPrompt)
	}
	args = append(args, message)
	if len(agent.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(agent.AllowedTools, ","))
	}
	if len(agent.DisallowedTools) > 0 {
		args = append(args, "--disallowed-tools", strings.Join(agent.DisallowedTools, ","))
	}
	if agent.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", agent.MaxTurns))
	}
	if agent.PermissionMode != "" {
		args = append(args, "--permission-mode", agent.PermissionMode)
	}
	return args
}

func RunClaude(ctx context.Context, cfg domain.Config, agent *domain.Agent, sessionID string, message string, resume bool, onEvent StreamCallback) error {
	args := BuildClaudeArgs(agent, sessionID, message, resume)
	if len(agent.MCPConfig) > 0 {
		tmpFile, err := os.CreateTemp("", "mcp-config-*.json")
		if err != nil {
			return fmt.Errorf("create mcp temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		enc := json.NewEncoder(tmpFile)
		if err := enc.Encode(agent.MCPConfig); err != nil {
			tmpFile.Close()
			return fmt.Errorf("write mcp config: %w", err)
		}
		tmpFile.Close()
		last := args[len(args)-1]
		args = args[:len(args)-1]
		args = append(args, "--mcp-config", tmpFile.Name(), last)
	}
	log.Printf("claude args: %v", args)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = cfg.WorkDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	var scanErr error
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		ev, err := ParseNDJSON(line)
		if err != nil {
			if cbErr := onEvent("parse_error", []byte(line)); cbErr != nil {
				scanErr = cbErr
				break
			}
			continue
		}
		if cbErr := onEvent(ev.Type, []byte(line)); cbErr != nil {
			scanErr = cbErr
			break
		}
	}
	if err := scanner.Err(); err != nil && scanErr == nil {
		scanErr = fmt.Errorf("scanner error: %w", err)
	}
	waitErr := cmd.Wait()
	if scanErr != nil {
		return scanErr
	}
	if waitErr != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return fmt.Errorf("claude exited: %s", stderr)
		}
		return fmt.Errorf("claude exited: %w", waitErr)
	}
	return nil
}

func FormatSSE(eventType, data string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
}
