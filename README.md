# claudio

Claude CLI proxy — manage AI agents with persistent sessions over your Claude subscription.

## Install

```bash
brew install bsantosio/tap/claudio
```

Or build from source:

```bash
go install ./cmd/claudio
```

## Commands

```
claudio            Interactive TUI (default)
claudio web        Start HTTP server + web UI (localhost:18080)
claudio mcp        Start MCP server over stdio
claudio prompt     Print AI-ready usage prompt
claudio version    Print version
```

### TUI (default)

```bash
claudio
```

Interactive terminal interface. Create agents, manage sessions, chat — all from the terminal.

### Web UI

```bash
claudio web
```

Starts the HTTP server and opens `http://localhost:18080` in your browser.

### MCP server

```bash
claudio mcp
```

JSON-RPC 2.0 over stdio. Add to your Claude Code or Cursor config:

```json
{
  "mcpServers": {
    "claudio": {
      "command": "claudio",
      "args": ["mcp"]
    }
  }
}
```

### AI prompt

```bash
claudio prompt
```

Outputs a detailed prompt describing all endpoints and usage — paste it into any AI for instant context.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `18080` | HTTP server port |
| `ADAPTER_API_KEY` | _(none)_ | Bearer token for API auth (optional) |
| `DEFAULT_MODEL` | `sonnet` | Default Claude model (`sonnet`, `haiku`, `opus`) |
| `WORK_DIR` | `.` | Working directory for Claude CLI |
| `DATA_DIR` | `./data` | SQLite database location |

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Server status + Claude auth check |
| `POST` | `/agents` | Create agent |
| `GET` | `/agents` | List agents |
| `GET` | `/agents/{id}` | Get agent |
| `PUT` | `/agents/{id}` | Update agent |
| `DELETE` | `/agents/{id}` | Delete agent |
| `POST` | `/agents/{id}/install` | Install agent to Claude Code |
| `DELETE` | `/agents/{id}/install` | Uninstall agent from Claude Code |
| `POST` | `/agents/{aid}/sessions` | Create session |
| `GET` | `/agents/{aid}/sessions` | List sessions for agent |
| `GET` | `/sessions/{sid}` | Get session |
| `DELETE` | `/sessions/{sid}` | Delete session |
| `POST` | `/sessions/{sid}/message` | Send message (SSE stream) |
| `GET` | `/sessions/{sid}/messages` | Message history |
| `GET` | `/v1/models` | List models (OpenAI compat) |
| `POST` | `/v1/chat/completions` | Chat completions (OpenAI compat) |

## Requirements

- [Claude CLI](https://docs.anthropic.com/en/docs/claude-cli) installed and authenticated (`claude auth login`)

## License

MIT
