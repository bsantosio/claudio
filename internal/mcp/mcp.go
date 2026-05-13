// Package mcp implements the Model Context Protocol server over stdio.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"claudio/internal/claude"
	"claudio/internal/domain"
	"claudio/internal/store"
)

// requireString extracts a required string field from args, returning an error
// if the key is missing, not a string, or blank after trimming whitespace.
func requireString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s cannot be empty", key)
	}
	return s, nil
}

// optionalString extracts an optional string field from args. Returns "" when
// the key is absent or the value is not a string.
func optionalString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	rpcParseError     = -32700
	rpcMethodNotFound = -32601
)

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": "Error: " + msg},
		},
		"isError": true,
	}
}

func mcpTools() []map[string]any {
	tools := []mcpTool{
		{
			Name:        "create_agent",
			Description: "Create a specialized AI agent with a custom system prompt, model, and tool configuration. Use list_templates first to see pre-built agents. The system_prompt defines the agent's behavior — structure it with sections: Purpose, Constraints, Workflow, Examples.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":            map[string]any{"type": "string", "description": "Agent name"},
					"system_prompt":   map[string]any{"type": "string", "description": "System prompt defining the agent's behavior"},
					"model":           map[string]any{"type": "string", "description": "Model to use: sonnet, haiku, opus", "default": "sonnet"},
					"allowed_tools":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Claude CLI tools the agent can use (e.g. Bash, Read, Edit)"},
					"max_turns":       map[string]any{"type": "integer", "description": "Maximum conversation turns per message"},
					"permission_mode": map[string]any{"type": "string", "description": "Permission mode: plan, auto-accept, or bypass"},
				},
				"required": []string{"name", "system_prompt"},
			},
		},
		{Name: "list_agents", Description: "List all configured agents", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "get_agent", Description: "Get details of a specific agent", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID"}}, "required": []string{"agent_id"}}},
		{Name: "delete_agent", Description: "Delete an agent (must have no active sessions)", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID"}}, "required": []string{"agent_id"}}},
		{Name: "create_session", Description: "Create a new conversation session for an agent", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID to create session for"}, "name": map[string]any{"type": "string", "description": "Optional session name"}}, "required": []string{"agent_id"}}},
		{Name: "list_sessions", Description: "List all sessions for an agent", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID"}}, "required": []string{"agent_id"}}},
		{Name: "send_message", Description: "Send a message to a session and get the AI response. The agent processes the message using Claude CLI with its configured system prompt, tools, and permissions. Returns the full response text. Use for multi-turn conversations — the session maintains context across messages.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID"}, "content": map[string]any{"type": "string", "description": "Message content to send"}}, "required": []string{"session_id", "content"}}},
		{Name: "get_messages", Description: "Get the conversation history for a session. Messages are read from Claude CLI's session JSONL files. Returns an array of {role, content} objects in chronological order.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID"}}, "required": []string{"session_id"}}},
		{Name: "delete_session", Description: "Delete a conversation session", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID"}}, "required": []string{"session_id"}}},
		{Name: "install_agent", Description: "Install an agent to Claude Code by generating a .md file in .claude/agents/. After installation, the agent is available as a sub-agent in Claude Code (invoked with @agent-name). The file includes the model config, allowed tools, and the full system prompt.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID to install"}}, "required": []string{"agent_id"}}},
		{Name: "uninstall_agent", Description: "Remove an agent from Claude Code by deleting its .md file from .claude/agents/.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID to uninstall"}}, "required": []string{"agent_id"}}},
		{Name: "list_templates", Description: "List pre-built agent templates with recommended system prompts, models, and tools. Use this to discover ready-made agents before creating custom ones.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "generate_agent", Description: "Generate an agent configuration using AI. Describe what you want the agent to do in natural language, and Claude will create an optimized agent config with the best system prompt, model, and tools. Review the result before creating.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"description": map[string]any{"type": "string", "description": "Natural language description of what you want the agent to do"}}, "required": []string{"description"}}},
	}
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		result[i] = map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema}
	}
	return result
}

// HandleMCPRequest dispatches an MCP JSON-RPC request using a nil runner
// (falls back to claude.RunClaude for tools that invoke Claude).
func HandleMCPRequest(req JSONRPCRequest, cfg domain.Config, st *store.Store) *JSONRPCResponse {
	return HandleMCPRequestWithRunner(req, cfg, st, nil)
}

// HandleMCPRequestWithRunner dispatches an MCP JSON-RPC request, passing an
// explicit runner to tool handlers. A nil runner uses claude.RunClaude.
func HandleMCPRequestWithRunner(req JSONRPCRequest, cfg domain.Config, st *store.Store, runner claude.Runner) *JSONRPCResponse {
	if req.ID == nil && req.Method != "" {
		handleNotification(req)
		return nil
	}
	resp := &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":        "claudio",
				"version":     domain.Version,
				"description": "Claude CLI proxy with agent management. Commands: `claudio` (TUI), `claudio web` (HTTP + web UI on port 18080), `claudio mcp` (MCP server, stdio), `claudio prompt` (AI usage prompt). Create named agents with custom system prompts and tools, open persistent sessions, chat with streaming. Install: brew install bsantosio/tap/claudio",
			},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		resp.Result = handleToolsCallWithRunner(req, cfg, st, runner)
	default:
		resp.Error = &RPCError{Code: rpcMethodNotFound, Message: "method not found: " + req.Method}
	}
	return resp
}

func handleNotification(req JSONRPCRequest) {
	switch req.Method {
	case "notifications/initialized":
		log.Printf("mcp: client initialized")
	default:
		log.Printf("mcp: unknown notification: %s", req.Method)
	}
}

func handleToolsCallWithRunner(req JSONRPCRequest, cfg domain.Config, st *store.Store, runner claude.Runner) map[string]any {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return toolError("invalid params: " + err.Error())
	}
	var args map[string]any
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return toolError("invalid arguments: " + err.Error())
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	switch params.Name {
	case "create_agent":
		return toolCreateAgent(args, cfg, st)
	case "list_agents":
		return toolListAgents(st)
	case "get_agent":
		return toolGetAgent(args, st)
	case "delete_agent":
		return toolDeleteAgent(args, st)
	case "create_session":
		return toolCreateSession(args, st)
	case "list_sessions":
		return toolListSessions(args, st)
	case "send_message":
		return toolSendMessage(args, cfg, st, runner)
	case "get_messages":
		return toolGetMessages(args, cfg)
	case "delete_session":
		return toolDeleteSession(args, st)
	case "install_agent":
		return toolInstallAgent(args, cfg, st)
	case "uninstall_agent":
		return toolUninstallAgent(args, cfg, st)
	case "list_templates":
		return toolListTemplates()
	case "generate_agent":
		return toolGenerateAgent(args, cfg, st, runner)
	default:
		return toolError("unknown tool: " + params.Name)
	}
}

func toolCreateAgent(args map[string]any, cfg domain.Config, st *store.Store) map[string]any {
	name, err := requireString(args, "name")
	if err != nil {
		return toolError(err.Error())
	}
	systemPrompt, err := requireString(args, "system_prompt")
	if err != nil {
		return toolError(err.Error())
	}
	model := optionalString(args, "model")
	permissionMode := optionalString(args, "permission_mode")
	var allowedTools []string
	if raw, ok := args["allowed_tools"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				allowedTools = append(allowedTools, s)
			}
		}
	}
	var maxTurns int
	if v, ok := args["max_turns"].(float64); ok {
		maxTurns = int(v)
	}
	input := domain.Agent{Name: name, SystemPrompt: systemPrompt, Model: model, AllowedTools: allowedTools, MaxTurns: maxTurns, PermissionMode: permissionMode}
	agent, err := st.CreateAgent(input, cfg.DefaultModel)
	if err != nil {
		return toolError(err.Error())
	}
	b, _ := json.Marshal(agent)
	return toolResult(string(b))
}

func toolListAgents(st *store.Store) map[string]any {
	agents, err := st.ListAgents()
	if err != nil {
		return toolError(err.Error())
	}
	b, _ := json.Marshal(agents)
	return toolResult(string(b))
}

func toolGetAgent(args map[string]any, st *store.Store) map[string]any {
	id, err := requireString(args, "agent_id")
	if err != nil {
		return toolError(err.Error())
	}
	agent, ok := st.GetAgent(id)
	if !ok {
		return toolError("agent not found: " + id)
	}
	b, _ := json.Marshal(agent)
	return toolResult(string(b))
}

func toolDeleteAgent(args map[string]any, st *store.Store) map[string]any {
	id, err := requireString(args, "agent_id")
	if err != nil {
		return toolError(err.Error())
	}
	if err := st.DeleteAgent(id); err != nil {
		return toolError(err.Error())
	}
	return toolResult(`{"deleted":true,"id":"` + id + `"}`)
}

func toolCreateSession(args map[string]any, st *store.Store) map[string]any {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return toolError(err.Error())
	}
	name := optionalString(args, "name")
	if _, ok := st.GetAgent(agentID); !ok {
		return toolError("agent not found: " + agentID)
	}
	sess, err := st.CreateSession(agentID, name)
	if err != nil {
		return toolError(err.Error())
	}
	b, _ := json.Marshal(sess)
	return toolResult(string(b))
}

func toolListSessions(args map[string]any, st *store.Store) map[string]any {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return toolError(err.Error())
	}
	sessions, err := st.ListSessionsByAgent(agentID)
	if err != nil {
		return toolError(err.Error())
	}
	b, _ := json.Marshal(sessions)
	return toolResult(string(b))
}

func toolSendMessage(args map[string]any, cfg domain.Config, st *store.Store, runner claude.Runner) map[string]any {
	sessionID, err := requireString(args, "session_id")
	if err != nil {
		return toolError(err.Error())
	}
	content, err := requireString(args, "content")
	if err != nil {
		return toolError(err.Error())
	}
	sess, ok := st.GetSession(sessionID)
	if !ok {
		return toolError("session not found: " + sessionID)
	}
	agent, ok := st.GetAgent(sess.AgentID)
	if !ok {
		return toolError("agent not found for session")
	}
	mu := st.Mutexes.Get(sess.ID)
	mu.Lock()
	defer mu.Unlock()
	resume := sess.TurnCount > 0
	sess.TurnCount++
	sess.LastActive = time.Now().UTC().Format(time.RFC3339Nano)
	var responseText strings.Builder
	if runner == nil {
		runner = claude.Runner(claude.RunClaude)
	}
	ctx := context.Background()
	err = runner(ctx, cfg, agent, sess.ID, content, resume, func(eventType string, data []byte) error {
		switch eventType {
		case "assistant":
			ev, err := claude.ParseNDJSON(string(data))
			if err != nil {
				return nil
			}
			text, err := claude.ExtractText(ev)
			if err != nil || text == "" {
				return nil
			}
			responseText.WriteString(text)
		case "result":
			var resultEv struct {
				Result    string  `json:"result"`
				TotalCost float64 `json:"total_cost_usd"`
			}
			if err := json.Unmarshal(data, &resultEv); err != nil {
				return nil
			}
			if responseText.Len() == 0 && resultEv.Result != "" {
				responseText.WriteString(resultEv.Result)
			}
		}
		return nil
	})
	if err != nil {
		return toolError("claude error: " + err.Error())
	}
	text := responseText.String()
	if err := st.SaveSession(sess); err != nil {
		log.Printf("mcp: failed to save session: %v", err)
	}
	return toolResult(text)
}

func toolGetMessages(args map[string]any, cfg domain.Config) map[string]any {
	sessionID, err := requireString(args, "session_id")
	if err != nil {
		return toolError(err.Error())
	}
	msgs, err := store.ReadSessionMessages(cfg.WorkDir, sessionID)
	if err != nil {
		return toolError(err.Error())
	}
	b, _ := json.Marshal(msgs)
	return toolResult(string(b))
}

func toolDeleteSession(args map[string]any, st *store.Store) map[string]any {
	id, err := requireString(args, "session_id")
	if err != nil {
		return toolError(err.Error())
	}
	if err := st.DeleteSession(id); err != nil {
		return toolError(err.Error())
	}
	return toolResult(`{"deleted":true,"id":"` + id + `"}`)
}

func toolInstallAgent(args map[string]any, cfg domain.Config, st *store.Store) map[string]any {
	id, err := requireString(args, "agent_id")
	if err != nil {
		return toolError(err.Error())
	}
	agent, ok := st.GetAgent(id)
	if !ok {
		return toolError("agent not found: " + id)
	}
	if err := domain.InstallAgent(agent, cfg.WorkDir); err != nil {
		return toolError(err.Error())
	}
	filename := domain.SanitizeName(agent.Name) + ".md"
	return toolResult(`{"installed":true,"agent":"` + agent.Name + `","path":".claude/agents/` + filename + `"}`)
}

func toolUninstallAgent(args map[string]any, cfg domain.Config, st *store.Store) map[string]any {
	id, err := requireString(args, "agent_id")
	if err != nil {
		return toolError(err.Error())
	}
	agent, ok := st.GetAgent(id)
	if !ok {
		return toolError("agent not found: " + id)
	}
	if err := domain.UninstallAgent(agent, cfg.WorkDir); err != nil {
		return toolError(err.Error())
	}
	return toolResult(`{"uninstalled":true,"agent":"` + agent.Name + `"}`)
}

func toolListTemplates() map[string]any {
	b, _ := json.Marshal(domain.AgentTemplates)
	return toolResult(string(b))
}

func toolGenerateAgent(args map[string]any, cfg domain.Config, st *store.Store, runner claude.Runner) map[string]any {
	description, err := requireString(args, "description")
	if err != nil {
		return toolError(err.Error())
	}

	generatorAgent := &domain.Agent{
		Model:        "sonnet",
		SystemPrompt: domain.AgentGeneratorPrompt,
	}
	sessionID, _ := domain.NewUUID()

	var result strings.Builder
	if runner == nil {
		runner = claude.Runner(claude.RunClaude)
	}

	ctx := context.Background()
	err = runner(ctx, cfg, generatorAgent, sessionID, description, false, func(eventType string, data []byte) error {
		if eventType == "result" {
			var ev struct {
				Result string `json:"result"`
			}
			if json.Unmarshal(data, &ev) == nil {
				result.WriteString(ev.Result)
			}
		}
		return nil
	})
	if err != nil {
		return toolError("generation failed: " + err.Error())
	}

	text := strings.TrimSpace(result.String())
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	return toolResult(text)
}

// RunMCPServer runs the MCP server reading from r and writing to w, using
// claude.RunClaude as the runner.
func RunMCPServer(cfg domain.Config, st *store.Store, r io.Reader, w io.Writer) error {
	return RunMCPServerWithRunner(cfg, st, r, w, nil)
}

// RunMCPServerWithRunner runs the MCP server with an explicit runner. A nil
// runner falls back to claude.RunClaude.
func RunMCPServerWithRunner(cfg domain.Config, st *store.Store, r io.Reader, w io.Writer, runner claude.Runner) error {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 10*1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := &JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &RPCError{Code: rpcParseError, Message: "parse error: " + err.Error()},
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintln(w, string(data))
			continue
		}
		resp := HandleMCPRequestWithRunner(req, cfg, st, runner)
		if resp == nil {
			continue
		}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("mcp: marshal error: %v", err)
			continue
		}
		fmt.Fprintln(w, string(data))
	}
	return scanner.Err()
}
