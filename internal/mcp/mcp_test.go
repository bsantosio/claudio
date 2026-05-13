package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/mcp"
	"claudio/internal/store"
)

func newMCPStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mcpRequest(method string, params any) mcp.JSONRPCRequest {
	var raw json.RawMessage
	if params != nil {
		b, _ := json.Marshal(params)
		raw = json.RawMessage(b)
	}
	return mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  raw,
	}
}

func mcpNotification(method string, params any) mcp.JSONRPCRequest {
	var raw json.RawMessage
	if params != nil {
		b, _ := json.Marshal(params)
		raw = json.RawMessage(b)
	}
	return mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
	}
}

func extractMCPContent(t *testing.T, result any) string {
	t.Helper()
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("extractMCPContent: expected map, got %T", result)
	}
	contents, ok := m["content"].([]map[string]any)
	if !ok {
		t.Fatalf("extractMCPContent: content is %T, not []map[string]any", m["content"])
	}
	if len(contents) == 0 {
		t.Fatal("extractMCPContent: content slice is empty")
	}
	text, _ := contents[0]["text"].(string)
	return text
}

func TestMCP_Initialize_ReturnsServerInfo(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "0.0.1"},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion 2024-11-05, got %v", result["protocolVersion"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected serverInfo map, got %T", result["serverInfo"])
	}
	if serverInfo["name"] != "claudio" {
		t.Errorf("expected serverInfo.name claudio, got %v", serverInfo["name"])
	}
}

func TestMCP_NotificationsInitialized_ReturnsNil(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpNotification("notifications/initialized", nil)
	resp := mcp.HandleMCPRequest(req, cfg, st)
	if resp != nil {
		t.Errorf("expected nil for notification, got %+v", resp)
	}
}

func TestMCP_ToolsList_ReturnsAllTools(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("tools/list", nil)
	resp := mcp.HandleMCPRequest(req, cfg, st)
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected failure: %+v", resp)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	tools, ok := result["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("expected tools slice, got %T", result["tools"])
	}
	expectedTools := []string{
		"create_agent", "list_agents", "get_agent", "delete_agent",
		"create_session", "list_sessions", "send_message", "get_messages", "delete_session",
		"install_agent", "uninstall_agent", "list_templates", "generate_agent",
	}
	if len(tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(tools))
	}
	nameSet := make(map[string]bool)
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		nameSet[name] = true
	}
	for _, expected := range expectedTools {
		if !nameSet[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestMCP_ToolsCall_CreateAgent_Success(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("tools/call", map[string]any{
		"name": "create_agent",
		"arguments": map[string]any{
			"name":          "test-agent",
			"system_prompt": "You are helpful.",
		},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected failure: %+v", resp)
	}
	content := extractMCPContent(t, resp.Result)
	if !strings.Contains(content, "test-agent") {
		t.Errorf("expected content to contain agent name, got: %s", content)
	}
	agents := st.ListAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent in store, got %d", len(agents))
	}
}

func TestMCP_ToolsCall_ListAgents_Empty(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("tools/call", map[string]any{
		"name":      "list_agents",
		"arguments": map[string]any{},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	content := extractMCPContent(t, resp.Result)
	if !strings.Contains(content, "[]") {
		t.Errorf("expected empty array in content, got: %s", content)
	}
}

func TestMCP_ToolsCall_GetAgent_NotFound(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("tools/call", map[string]any{
		"name":      "get_agent",
		"arguments": map[string]any{"agent_id": "nonexistent-id"},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	result, _ := resp.Result.(map[string]any)
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for not found agent")
	}
}

func TestMCP_ToolsCall_DeleteAgent_Success(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	created, _ := st.CreateAgent(domain.Agent{Name: "to-delete", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	req := mcpRequest("tools/call", map[string]any{
		"name":      "delete_agent",
		"arguments": map[string]any{"agent_id": created.ID},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	content := extractMCPContent(t, resp.Result)
	if !strings.Contains(content, "deleted") {
		t.Errorf("expected 'deleted' in content, got: %s", content)
	}
	_, ok := st.GetAgent(created.ID)
	if ok {
		t.Error("expected agent to be deleted from store")
	}
}

func TestMCP_ToolsCall_CreateSession_Success(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	agent, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	req := mcpRequest("tools/call", map[string]any{
		"name":      "create_session",
		"arguments": map[string]any{"agent_id": agent.ID, "name": "my-session"},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	content := extractMCPContent(t, resp.Result)
	if !strings.Contains(content, agent.ID) {
		t.Errorf("expected agent_id in session response, got: %s", content)
	}
}

func TestMCP_ToolsCall_SendMessage_Success(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet", WorkDir: t.TempDir()}
	agent, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "")
	mcp.RunnerOverride = func(ctx context.Context, c domain.Config, a *domain.Agent, sessionID, message string, resume bool, onEvent claude.StreamCallback) error {
		data := `{"type":"result","subtype":"success","result":"mock response","session_id":"` + sessionID + `","total_cost_usd":0}`
		return onEvent("result", []byte(data))
	}
	defer func() { mcp.RunnerOverride = nil }()
	req := mcpRequest("tools/call", map[string]any{
		"name":      "send_message",
		"arguments": map[string]any{"session_id": sess.ID, "content": "hello"},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	content := extractMCPContent(t, resp.Result)
	if !strings.Contains(content, "mock response") {
		t.Errorf("expected mock response in content, got: %s", content)
	}
}

func TestMCP_ToolsCall_SendMessage_SessionNotFound(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("tools/call", map[string]any{
		"name":      "send_message",
		"arguments": map[string]any{"session_id": "ghost-session", "content": "hello"},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	result, _ := resp.Result.(map[string]any)
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for unknown session")
	}
}

func TestMCP_ToolsCall_DeleteSession_Success(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	agent, _ := st.CreateAgent(domain.Agent{Name: "a", SystemPrompt: "sp", Model: "sonnet"}, "sonnet")
	sess, _ := st.CreateSession(agent.ID, "")
	req := mcpRequest("tools/call", map[string]any{
		"name":      "delete_session",
		"arguments": map[string]any{"session_id": sess.ID},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	content := extractMCPContent(t, resp.Result)
	if !strings.Contains(content, "deleted") {
		t.Errorf("expected 'deleted' in content, got: %s", content)
	}
}

func TestMCP_ToolsCall_UnknownTool_ReturnsIsError(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("tools/call", map[string]any{
		"name":      "nonexistent_tool",
		"arguments": map[string]any{},
	})
	resp := mcp.HandleMCPRequest(req, cfg, st)
	result, _ := resp.Result.(map[string]any)
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for unknown tool")
	}
}

func TestMCP_UnknownMethod_ReturnsRPCError(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	req := mcpRequest("unknown/method", nil)
	resp := mcp.HandleMCPRequest(req, cfg, st)
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Error == nil {
		t.Error("expected RPC error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected method-not-found code -32601, got %d", resp.Error.Code)
	}
}

func TestMCP_RunMCPServer_ProcessesMultipleRequests(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	initReq, _ := json.Marshal(mcpRequest("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
	}))
	toolsReq, _ := json.Marshal(mcpRequest("tools/list", nil))
	input := string(initReq) + "\n" + string(toolsReq) + "\n"
	reader := strings.NewReader(input)
	var buf strings.Builder
	err := mcp.RunMCPServer(cfg, st, reader, &buf)
	if err != nil {
		t.Fatalf("RunMCPServer returned error: %v", err)
	}
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 response lines, got %d: %q", len(lines), output)
	}
	for i, line := range lines {
		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Errorf("line %d is not valid JSON: %v — %q", i, err, line)
		}
		if resp.JSONRPC != "2.0" {
			t.Errorf("line %d: expected jsonrpc 2.0, got %q", i, resp.JSONRPC)
		}
	}
}

func TestMCP_RunMCPServer_SkipsNotifications(t *testing.T) {
	st := newMCPStore(t)
	cfg := domain.Config{DefaultModel: "sonnet"}
	notif, _ := json.Marshal(mcpNotification("notifications/initialized", nil))
	toolsReq, _ := json.Marshal(mcpRequest("tools/list", nil))
	input := string(notif) + "\n" + string(toolsReq) + "\n"
	reader := strings.NewReader(input)
	var buf strings.Builder
	err := mcp.RunMCPServer(cfg, st, reader, &buf)
	if err != nil {
		t.Fatalf("RunMCPServer returned error: %v", err)
	}
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 response line, got %d: %q", len(lines), output)
	}
}
