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

var RunnerOverride claude.Runner

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
			Description: "Create a specialized AI agent with a custom system prompt, model, and tool configuration",
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
		{Name: "send_message", Description: "Send a message to a session and get the AI response. The agent processes the message using Claude CLI with its configured tools and permissions.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID"}, "content": map[string]any{"type": "string", "description": "Message content to send"}}, "required": []string{"session_id", "content"}}},
		{Name: "get_messages", Description: "Get the message history for a session", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID"}}, "required": []string{"session_id"}}},
		{Name: "delete_session", Description: "Delete a conversation session", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"session_id": map[string]any{"type": "string", "description": "Session ID"}}, "required": []string{"session_id"}}},
		{Name: "install_agent", Description: "Install an agent to Claude Code by generating a .md file in .claude/agents/. After installation, the agent is available as a sub-agent in Claude Code.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID to install"}}, "required": []string{"agent_id"}}},
		{Name: "uninstall_agent", Description: "Remove an agent from Claude Code by deleting its .md file from .claude/agents/.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"agent_id": map[string]any{"type": "string", "description": "Agent ID to uninstall"}}, "required": []string{"agent_id"}}},
	}
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		result[i] = map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema}
	}
	return result
}

func HandleMCPRequest(req JSONRPCRequest, cfg domain.Config, st *store.Store) *JSONRPCResponse {
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
				"description": "Proxy HTTP that manages specialized AI agents and persistent sessions over Claude CLI. Create agents with custom system prompts and tools, open sessions, and send messages. Any local app can use Claude via this proxy.",
			},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		resp.Result = handleToolsCall(req, cfg, st)
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

func handleToolsCall(req JSONRPCRequest, cfg domain.Config, st *store.Store) map[string]any {
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
		return toolSendMessage(args, cfg, st)
	case "get_messages":
		return toolGetMessages(args, cfg)
	case "delete_session":
		return toolDeleteSession(args, st)
	case "install_agent":
		return toolInstallAgent(args, cfg, st)
	case "uninstall_agent":
		return toolUninstallAgent(args, cfg, st)
	default:
		return toolError("unknown tool: " + params.Name)
	}
}

func toolCreateAgent(args map[string]any, cfg domain.Config, st *store.Store) map[string]any {
	name, _ := args["name"].(string)
	systemPrompt, _ := args["system_prompt"].(string)
	model, _ := args["model"].(string)
	permissionMode, _ := args["permission_mode"].(string)
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
	agents := st.ListAgents()
	b, _ := json.Marshal(agents)
	return toolResult(string(b))
}

func toolGetAgent(args map[string]any, st *store.Store) map[string]any {
	id, _ := args["agent_id"].(string)
	agent, ok := st.GetAgent(id)
	if !ok {
		return toolError("agent not found: " + id)
	}
	b, _ := json.Marshal(agent)
	return toolResult(string(b))
}

func toolDeleteAgent(args map[string]any, st *store.Store) map[string]any {
	id, _ := args["agent_id"].(string)
	if err := st.DeleteAgent(id); err != nil {
		return toolError(err.Error())
	}
	return toolResult(`{"deleted":true,"id":"` + id + `"}`)
}

func toolCreateSession(args map[string]any, st *store.Store) map[string]any {
	agentID, _ := args["agent_id"].(string)
	name, _ := args["name"].(string)
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
	agentID, _ := args["agent_id"].(string)
	sessions := st.ListSessionsByAgent(agentID)
	b, _ := json.Marshal(sessions)
	return toolResult(string(b))
}

func toolSendMessage(args map[string]any, cfg domain.Config, st *store.Store) map[string]any {
	sessionID, _ := args["session_id"].(string)
	content, _ := args["content"].(string)
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
	runner := claude.Runner(claude.RunClaude)
	if RunnerOverride != nil {
		runner = RunnerOverride
	}
	ctx := context.Background()
	err := runner(ctx, cfg, agent, sess.ID, content, resume, func(eventType string, data []byte) error {
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
	st.SaveSession(sess)
	return toolResult(text)
}

func toolGetMessages(args map[string]any, cfg domain.Config) map[string]any {
	sessionID, _ := args["session_id"].(string)
	msgs, err := store.ReadSessionMessages(cfg.WorkDir, sessionID)
	if err != nil {
		return toolError(err.Error())
	}
	b, _ := json.Marshal(msgs)
	return toolResult(string(b))
}

func toolDeleteSession(args map[string]any, st *store.Store) map[string]any {
	id, _ := args["session_id"].(string)
	if err := st.DeleteSession(id); err != nil {
		return toolError(err.Error())
	}
	return toolResult(`{"deleted":true,"id":"` + id + `"}`)
}

func toolInstallAgent(args map[string]any, cfg domain.Config, st *store.Store) map[string]any {
	id, _ := args["agent_id"].(string)
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
	id, _ := args["agent_id"].(string)
	agent, ok := st.GetAgent(id)
	if !ok {
		return toolError("agent not found: " + id)
	}
	if err := domain.UninstallAgent(agent, cfg.WorkDir); err != nil {
		return toolError(err.Error())
	}
	return toolResult(`{"uninstalled":true,"agent":"` + agent.Name + `"}`)
}

func RunMCPServer(cfg domain.Config, st *store.Store, r io.Reader, w io.Writer) error {
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
		resp := HandleMCPRequest(req, cfg, st)
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
