# OpenCode Integration Reference

> For questions about OpenCode's CLI, SDK, Server, or Plugins, refer to:
> - CLI: https://opencode.ai/docs/cli/
> - SDK: https://opencode.ai/docs/sdk/
> - Server: https://opencode.ai/docs/server/
> - Plugins: https://opencode.ai/docs/plugins/

## Execution Modes

All modes support plugins. Choose based on your use case:

| Mode | Command | Use Case | Notes |
|------|---------|----------|-------|
| **Interactive TUI** | `opencode` | Human-in-the-loop | Combo server+client, plugins work |
| **Attach to Server** | `opencode attach <url>` | Connect TUI to existing server | Shares MCP servers |
| **Headless CLI** | `opencode run "prompt"` | Scripting/automation | Can attach to server with `--attach` |
| **Server Only** | `opencode serve` | API access, multiple clients | Default port 4096 |
| **Web Interface** | `opencode web` | Browser access | Opens web UI |

## Gastown Use Cases

| Gastown Operation | OpenCode Approach | Why |
|-------------------|-------------------|-----|
| Start Mayor agent | `opencode --port <port> --session <id>` in tmux | Interactive, plugins fire, same as Claude |
| Send nudge to Mayor | `tmux send-keys` OR SDK `session.prompt()` | tmux is simpler for existing infra |
| Run headless task | `opencode run "task" --attach <server>` | Avoids MCP cold boot |
| Polecat work | `opencode run "task"` | Headless execution |
| Check costs | Plugin `session.idle` hook → `gt costs record` | Automatic tracking |

## SDK vs CLI Decision

**Use SDK when**:
- Building programmatic orchestration
- Need type-safe session management  
- Creating custom tools/integrations
- Want to avoid process spawning overhead

**Use CLI when**:
- Running in tmux (human-visible)
- Simple scripting
- One-off tasks
- Leveraging existing shell infrastructure

## Key SDK APIs

```javascript
import { createOpencode, createOpencodeClient } from "@opencode-ai/sdk"

// Start server + client
const { client, server } = await createOpencode({ port: 4096 })

// Connect to existing server
const client = createOpencodeClient({ baseUrl: "http://localhost:4096" })

// Session management
const session = await client.session.create({ body: { title: "Mayor" } })
await client.session.prompt({
  path: { id: session.id },
  body: {
    parts: [{ type: "text", text: "Your task..." }]
  }
})

// Inject context without AI response (noReply)
await client.session.prompt({
  path: { id: session.id },
  body: {
    noReply: true,
    parts: [{ type: "text", text: "Context injection..." }]
  }
})
```

## Server HTTP APIs

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/session` | GET | List sessions |
| `/session` | POST | Create session |
| `/session/:id` | GET | Get session info |
| `/session/:id/message` | POST | Send prompt (with `noReply` option) |
| `/session/:id/abort` | POST | Cancel running request |
| `/session/:id/fork` | POST | Fork session from message |

## Plugin Events (for gastown.js)

| Event | Fires When | Gastown Use |
|-------|------------|-------------|
| `session.created` | New session starts | `gt prime`, `gt mail check` |
| `message.updated` | Response complete | Check mail for interactive roles |
| `session.idle` | No activity | `gt costs record` |
| `session.compacted` | Context compaction | Archive to `.beads` |
| `session.error` | Error occurs | Log and escalate |

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `OPENCODE_SERVER_PASSWORD` | HTTP basic auth password |
| `OPENCODE_SERVER_USERNAME` | HTTP basic auth username (default: opencode) |
| `OPENCODE_PERMISSION` | Auto-approve permissions |
| `OPENCODE_CONFIG` | Path to config file |

## CLI Flags Reference

### `opencode` (TUI)
- `--continue/-c` - Resume last session
- `--session/-s <id>` - Resume specific session
- `--prompt` - Initial prompt
- `--port` - Server port
- `--agent` - Use specific agent

### `opencode run` (Headless)
- `--attach <url>` - Connect to existing server
- `--format` - Output format (json, text)
- `--session/-s` - Use existing session
- `--model/-m` - Override model

### `opencode serve` (Server)
- `--port` - Listen port (default: 4096)
- `--hostname` - Bind address (default: 127.0.0.1)
- `--cors` - CORS origins (repeatable)
