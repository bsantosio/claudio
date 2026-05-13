package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"claudio/internal/domain"
)

func TestParseNDJSON_TextDelta(t *testing.T) {
	line := `{"type":"assistant","message":{"id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"Hello, world!"}]},"session_id":"sess-123"}`
	ev, err := ParseNDJSON(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "assistant" {
		t.Errorf("expected type 'assistant', got %q", ev.Type)
	}
	text, err := ExtractText(ev)
	if err != nil {
		t.Fatalf("ExtractText error: %v", err)
	}
	if text != "Hello, world!" {
		t.Errorf("expected text 'Hello, world!', got %q", text)
	}
}

func TestParseNDJSON_ResultEvent(t *testing.T) {
	line := `{"type":"result","subtype":"success","result":"Full response text","session_id":"sess-123","total_cost_usd":0.001}`
	ev, err := ParseNDJSON(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "result" {
		t.Errorf("expected type 'result', got %q", ev.Type)
	}
	if ev.Subtype != "success" {
		t.Errorf("expected subtype 'success', got %q", ev.Subtype)
	}
	if string(ev.Result) == "" || string(ev.Result) == "null" {
		t.Error("expected Result to be populated")
	}
}

func TestParseNDJSON_SystemInitEvent(t *testing.T) {
	line := `{"type":"system","subtype":"init","session_id":"sess-abc","tools":[]}`
	ev, err := ParseNDJSON(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "system" {
		t.Errorf("expected type 'system', got %q", ev.Type)
	}
	if ev.Subtype != "init" {
		t.Errorf("expected subtype 'init', got %q", ev.Subtype)
	}
	if ev.SessionID != "sess-abc" {
		t.Errorf("expected session_id 'sess-abc', got %q", ev.SessionID)
	}
}

func TestParseNDJSON_UnknownEventType(t *testing.T) {
	line := `{"type":"unknown_future_type","data":{"foo":"bar"}}`
	ev, err := ParseNDJSON(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "unknown_future_type" {
		t.Errorf("expected type 'unknown_future_type', got %q", ev.Type)
	}
	if ev.RawLine == "" {
		t.Error("expected RawLine to be set for unknown events")
	}
}

func TestParseNDJSON_InvalidJSON(t *testing.T) {
	_, err := ParseNDJSON("not valid json{{{")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestBuildClaudeArgs_BasicCommand(t *testing.T) {
	agent := &domain.Agent{
		ID:           "agent-1",
		SystemPrompt: "You are helpful.",
		Model:        "sonnet",
	}
	args := BuildClaudeArgs(agent, "session-123", "Hello", false)
	mustContain := []string{"-p", "--verbose", "--session-id", "session-123", "--output-format", "stream-json", "--model", "sonnet", "--system-prompt", "You are helpful.", "Hello"}
	for _, expected := range mustContain {
		found := false
		for _, arg := range args {
			if arg == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected arg %q not found in %v", expected, args)
		}
	}
}

func TestBuildClaudeArgs_WithAllowedTools(t *testing.T) {
	agent := &domain.Agent{
		SystemPrompt: "You are helpful.",
		Model:        "sonnet",
		AllowedTools: []string{"Bash", "Read"},
	}
	args := BuildClaudeArgs(agent, "sess", "Hi", false)
	found := false
	for i, arg := range args {
		if arg == "--allowedTools" && i+1 < len(args) {
			if args[i+1] == "Bash,Read" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected --allowedTools Bash,Read in args %v", args)
	}
}

func TestBuildClaudeArgs_WithDisallowedTools(t *testing.T) {
	agent := &domain.Agent{
		SystemPrompt:    "You are helpful.",
		Model:           "sonnet",
		DisallowedTools: []string{"Bash"},
	}
	args := BuildClaudeArgs(agent, "sess", "Hi", false)
	found := false
	for i, arg := range args {
		if arg == "--disallowed-tools" && i+1 < len(args) {
			if args[i+1] == "Bash" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected --disallowed-tools Bash in args %v", args)
	}
}

func TestBuildClaudeArgs_WithMaxTurns(t *testing.T) {
	agent := &domain.Agent{
		SystemPrompt: "You are helpful.",
		Model:        "sonnet",
		MaxTurns:     5,
	}
	args := BuildClaudeArgs(agent, "sess", "Hi", false)
	found := false
	for i, arg := range args {
		if arg == "--max-turns" && i+1 < len(args) {
			if args[i+1] == "5" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected --max-turns 5 in args %v", args)
	}
}

func TestBuildClaudeArgs_WithPermissionMode(t *testing.T) {
	agent := &domain.Agent{
		SystemPrompt:   "You are helpful.",
		Model:          "sonnet",
		PermissionMode: "bypass",
	}
	args := BuildClaudeArgs(agent, "sess", "Hi", false)
	found := false
	for i, arg := range args {
		if arg == "--permission-mode" && i+1 < len(args) {
			if args[i+1] == "bypass" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected --permission-mode bypass in args %v", args)
	}
}

func TestSSEFormat_TextDelta(t *testing.T) {
	data := map[string]string{"text": "Hello"}
	jsonBytes, _ := json.Marshal(data)
	sse := FormatSSE("text_delta", string(jsonBytes))
	expected := "event: text_delta\ndata: " + string(jsonBytes) + "\n\n"
	if sse != expected {
		t.Errorf("expected SSE %q, got %q", expected, sse)
	}
}

func TestSSEFormat_ResultEvent(t *testing.T) {
	data := map[string]any{"text": "full response", "cost_usd": 0.001}
	jsonBytes, _ := json.Marshal(data)
	sse := FormatSSE("result", string(jsonBytes))
	if !strings.HasPrefix(sse, "event: result\n") {
		t.Errorf("SSE result event should start with 'event: result\\n', got %q", sse)
	}
	if !strings.HasSuffix(sse, "\n\n") {
		t.Errorf("SSE event should end with '\\n\\n'")
	}
}

func TestLargeLineScanner(t *testing.T) {
	longText := strings.Repeat("x", 512*1024)
	line := fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]},"session_id":"s"}`, longText)
	scanner := bufio.NewScanner(strings.NewReader(line))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	if !scanner.Scan() {
		t.Fatal("scanner failed to read large line")
	}
	scanned := scanner.Text()
	if len(scanned) != len(line) {
		t.Errorf("expected %d bytes, got %d", len(line), len(scanned))
	}
}
