package domain

const AIPrompt = `# claudio — Claude CLI Agent Proxy

You have access to claudio, a proxy that manages specialized AI agents with persistent sessions over Claude CLI.

## How to connect

### As MCP server (recommended for AI tools)
Add to your MCP config:
` + "```" + `json
{
  "mcpServers": {
    "claudio": {
      "command": "claudio",
      "args": ["mcp"]
    }
  }
}
` + "```" + `

### As HTTP API
Base URL: http://localhost:18080 (configurable via PORT env var)
Start manually: claudio web
Install as service: claudio service install (runs in background, starts at login)

---

## Models

| Alias | Model ID | Context | Description |
|-------|----------|---------|-------------|
| sonnet | claude-sonnet-4-6 | 200k | Fast, balanced — default |
| opus | claude-opus-4-7 | 200k | Most capable |
| haiku | claude-haiku-4-5 | 200k | Fastest, cheapest |
| claude-opus-4-6[1m] | claude-opus-4-6[1m] | 1M | Opus 4.6 with 1M context |

Use the alias (sonnet, opus, haiku) or the full model ID. OpenAI aliases also work: gpt-4 → opus, gpt-4o → sonnet, gpt-3.5-turbo → haiku.

---

## Agents API

### Create agent
` + "```" + `bash
curl -X POST http://localhost:18080/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "code-reviewer",
    "system_prompt": "You are an expert code reviewer. Review code for bugs, security issues, and best practices.",
    "model": "sonnet",
    "allowed_tools": ["Bash", "Read", "Grep"],
    "max_turns": 10,
    "permission_mode": "auto-accept"
  }'
` + "```" + `

Response (201):
` + "```" + `json
{
  "id": "a1b2c3d4-...",
  "name": "code-reviewer",
  "system_prompt": "You are an expert code reviewer...",
  "model": "sonnet",
  "allowed_tools": ["Bash", "Read", "Grep"],
  "max_turns": 10,
  "permission_mode": "auto-accept",
  "created_at": "2026-05-13T...",
  "updated_at": "2026-05-13T..."
}
` + "```" + `

### List agents
` + "```" + `bash
curl http://localhost:18080/agents
` + "```" + `

### Get agent
` + "```" + `bash
curl http://localhost:18080/agents/{id}
` + "```" + `

### Update agent
` + "```" + `bash
curl -X PUT http://localhost:18080/agents/{id} \
  -H "Content-Type: application/json" \
  -d '{"name": "new-name", "model": "opus"}'
` + "```" + `

### Delete agent
` + "```" + `bash
curl -X DELETE http://localhost:18080/agents/{id}
` + "```" + `

### Install agent to Claude Code
` + "```" + `bash
curl -X POST http://localhost:18080/agents/{id}/install
` + "```" + `
Creates a .md file in .claude/agents/ so the agent is available as a sub-agent (@agent-name).

### Uninstall agent from Claude Code
` + "```" + `bash
curl -X DELETE http://localhost:18080/agents/{id}/install
` + "```" + `

---

## Sessions API

### Create session
` + "```" + `bash
curl -X POST http://localhost:18080/agents/{agent_id}/sessions \
  -H "Content-Type: application/json" \
  -d '{"name": "my-session"}'
` + "```" + `

Response (201):
` + "```" + `json
{
  "id": "s1e2s3s4-...",
  "agent_id": "a1b2c3d4-...",
  "name": "my-session",
  "turn_count": 0,
  "created_at": "2026-05-13T...",
  "last_active": "2026-05-13T..."
}
` + "```" + `

### List sessions for agent
` + "```" + `bash
curl http://localhost:18080/agents/{agent_id}/sessions
` + "```" + `

### Get session
` + "```" + `bash
curl http://localhost:18080/sessions/{session_id}
` + "```" + `

### Delete session
` + "```" + `bash
curl -X DELETE http://localhost:18080/sessions/{session_id}
` + "```" + `

---

## Messages API

### Send message (SSE streaming)
` + "```" + `bash
curl -N -X POST http://localhost:18080/sessions/{session_id}/message \
  -H "Content-Type: application/json" \
  -d '{"content": "Review this function for bugs"}'
` + "```" + `

Response is a Server-Sent Events stream:
` + "```" + `
event: session_init
data: {"session_id": "s1e2s3s4-..."}

event: text_delta
data: {"text": "Looking at the function..."}

event: text_delta
data: {"text": " I found a potential null pointer issue."}

event: result
data: {"text": "full response", "cost_usd": 0.0756, "usage": {"input_tokens": 6, "output_tokens": 42, "cache_read_input_tokens": 32000, "cache_creation_input_tokens": 3000, "context_window": 200000}}
` + "```" + `

### Get message history
` + "```" + `bash
curl http://localhost:18080/sessions/{session_id}/messages
` + "```" + `

Response:
` + "```" + `json
[
  {"role": "user", "content": "Review this function"},
  {"role": "assistant", "content": "Looking at the function..."}
]
` + "```" + `

---

## OpenAI-Compatible API

### List models
` + "```" + `bash
curl http://localhost:18080/v1/models
` + "```" + `

### Chat completions (non-streaming)
` + "```" + `bash
curl -X POST http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "messages": [
      {"role": "system", "content": "You are helpful."},
      {"role": "user", "content": "Hello!"}
    ],
    "stream": false
  }'
` + "```" + `

Response:
` + "```" + `json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "model": "sonnet",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "Hello!"},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 35000, "completion_tokens": 12, "total_tokens": 35012}
}
` + "```" + `

### Chat completions (streaming)
` + "```" + `bash
curl -N -X POST http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "sonnet", "messages": [{"role": "user", "content": "Hello"}], "stream": true}'
` + "```" + `

Use X-Agent-Id header to route through a specific agent:
` + "```" + `bash
curl -X POST http://localhost:18080/v1/chat/completions \
  -H "X-Agent-Id: a1b2c3d4-..." \
  -H "Content-Type: application/json" \
  -d '{"model": "sonnet", "messages": [{"role": "user", "content": "Review this"}], "stream": false}'
` + "```" + `

---

## System

### Health check
` + "```" + `bash
curl http://localhost:18080/health
` + "```" + `

Response:
` + "```" + `json
{"status": "ok", "claude_auth": "authenticated", "version": "0.3.0", "claude_version": "2.1.140"}
` + "```" + `

### Authentication
Set ADAPTER_API_KEY env var to require Bearer token:
` + "```" + `bash
ADAPTER_API_KEY=my-secret claudio web
curl -H "Authorization: Bearer my-secret" http://localhost:18080/agents
` + "```" + `

---

## Workflow

1. Create an agent with a system prompt, model, and tools
2. Create a session for that agent
3. Send messages to the session — responses stream via SSE with token usage
4. Optionally install the agent to Claude Code for use as a sub-agent (@name)
5. Use the OpenAI-compatible endpoint for drop-in replacement in existing apps

---

## Writing Good Agent Prompts

Structure every agent prompt with these sections:
1. **Purpose** — What the agent does (1-2 sentences)
2. **Constraints** — MUST do / MUST NOT do rules
3. **Workflow** — Step-by-step process
4. **Output format** — How to structure responses

Tips: Be specific (name exact checks), set boundaries (what NOT to do), define output format, give examples. Use haiku for quick tasks, sonnet for balanced work, opus for complex reasoning.
`
