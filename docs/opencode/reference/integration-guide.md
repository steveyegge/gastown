# OpenCode Integration Guide

> **Purpose**: Complete reference for integrating with OpenCode  
> **Source of Truth**: [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode)  
> **Official Docs**: [opencode.ai/docs](https://opencode.ai/docs/)

This guide covers all integration methods with OpenCode: CLI, SDK, HTTP API, TUI, and Plugins.

---

## Quick Reference

| Method | Use Case | Language | Docs |
|--------|----------|----------|------|
| **CLI** | Interactive use, scripts, CI | Shell | [cli](#cli) |
| **SDK** | Programmatic control | TS/JS, Python, Go | [sdk](#sdk) |
| **HTTP API** | Remote control, microservices | Any | [api](#http-api) |
| **TUI** | Interactive terminal UI | - | [tui](#tui) |
| **Plugin** | Extend OpenCode behavior | JS/TS | [plugin](#plugins) |

---

## CLI

### Installation

```bash
npm install -g opencode-ai
```

### Basic Usage

```bash
# Interactive session
opencode

# Non-interactive (run and exit)
opencode run "your prompt here"

# Resume session
opencode --session <session-id>
opencode -c  # Resume last session

# List sessions
opencode session list

# Export/import
opencode export <session-id> > session.json
opencode import session.json
```

### Session Management

```bash
# Fork session (via TUI)
opencode --session <id>
# Then type: /fork

# Server mode (enables HTTP API)
opencode web           # Web server
opencode acp           # ACP protocol
opencode serve         # Generic server
```

### Configuration

```bash
# Location: ~/.config/opencode/opencode.jsonc
# Or project-level: .opencode/config.jsonc
```

---

## SDK

### Language Support

| Language | Package | Docs |
|----------|---------|------|
| **TypeScript/JS** | `@opencode-ai/sdk` | [npm](https://www.npmjs.com/package/@opencode-ai/sdk) |
| **Python** | `opencode` | [pypi](https://pypi.org/project/opencode/) |
| **Go** | `opencode-sdk-go` | [pkg.go.dev](https://pkg.go.dev/github.com/anomalyco/opencode-sdk-go) |

### TypeScript/JavaScript

```bash
npm install @opencode-ai/sdk
```

```typescript
import { createOpencode, createOpencodeClient } from "@opencode-ai/sdk"

// Start server and get client
const { client, server } = await createOpencode({
  hostname: "127.0.0.1",
  port: 4096,
})

// Or connect to existing server
const client = createOpencodeClient({ baseUrl: "http://localhost:4096" })

// Session management
const session = await client.session.create({ body: { title: "My session" } })
const sessions = await client.session.list()

// Send prompt
const result = await client.session.prompt({
  path: { id: session.id },
  body: {
    parts: [{ type: "text", text: "Hello!" }],
  },
})

// Context injection (no AI response)
await client.session.prompt({
  path: { id: session.id },
  body: {
    noReply: true,
    parts: [{ type: "text", text: "Context to inject" }],
  },
})

// Fork session
await client.session.fork({
  path: { id: session.id },
  body: { messageID: "msg_xyz" },
})

// Subscribe to events
const events = await client.event.subscribe()
for await (const event of events.stream) {
  console.log("Event:", event.type, event.properties)
}

// Cleanup
server.close()
```

### Python

```bash
pip install opencode
```

```python
from opencode import Opencode

client = Opencode()
session = client.session.create(title="My session")
result = client.session.prompt(session.id, "Hello!")
```

### Go

```bash
go get github.com/anomalyco/opencode-sdk-go
```

```go
import "github.com/anomalyco/opencode-sdk-go"

client := opencode.NewClient("http://localhost:4096")
session, _ := client.Session.Create(ctx, &opencode.SessionCreateParams{
    Title: "My session",
})
```

---

## HTTP API

Requires server mode: `opencode web` or `opencode serve`

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/session` | List sessions |
| `POST` | `/session` | Create session |
| `GET` | `/session/:id` | Get session |
| `DELETE` | `/session/:id` | Delete session |
| `POST` | `/session/:id/prompt` | Send prompt |
| `POST` | `/session/:id/fork` | Fork session |
| `GET` | `/session/:id/messages` | Get messages |
| `GET` | `/event` | SSE event stream |

### Examples

```bash
# Health check
curl http://localhost:4096/health

# List sessions
curl http://localhost:4096/session

# Create session
curl -X POST http://localhost:4096/session \
  -H "Content-Type: application/json" \
  -d '{"title": "My session"}'

# Send prompt
curl -X POST http://localhost:4096/session/{id}/prompt \
  -H "Content-Type: application/json" \
  -d '{"parts": [{"type": "text", "text": "Hello!"}]}'

# Fork at specific message
curl -X POST http://localhost:4096/session/{id}/fork \
  -H "Content-Type: application/json" \
  -d '{"messageID": "msg_xyz"}'

# Context injection (no response)
curl -X POST http://localhost:4096/session/{id}/prompt \
  -H "Content-Type: application/json" \
  -d '{"noReply": true, "parts": [{"type": "text", "text": "Context"}]}'
```

---

## TUI

OpenCode has a built-in terminal UI with commands:

### Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show help |
| `/fork` | Fork current session |
| `/compact` | Compact session context |
| `/clear` | Clear screen |
| `/models` | List available models |
| `/sessions` | List sessions |
| `/theme` | Change theme |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+C` | Cancel current operation |
| `Ctrl+D` | Exit |
| `Up/Down` | Scroll history |
| `Tab` | Autocomplete |

---

## Plugins

Plugins extend OpenCode's behavior via JavaScript event handlers.

### Location

```
.opencode/plugin/<name>.js
```

### Basic Structure

```javascript
export const MyPlugin = async ({ $, directory }) => {
  return {
    // Subscribe to events
    event: async ({ event }) => {
      console.log("Event:", event.type)
    },

    // Register custom tools
    tool: {
      myTool: {
        description: "My custom tool",
        parameters: { /* JSON schema */ },
        execute: async (params, context) => {
          return { result: "done" }
        },
      },
    },

    // Pre-compaction hook
    "experimental.session.compacting": async (input, output) => {
      output.context.push("Additional context for compaction")
    },
  }
}
```

### Plugin Return Object

Plugins return an object with **event handlers** and **hooks**:

- **`event`**: Subscribe to events (read-only notifications like `session.created`)
- **Named hooks**: Intercept and modify behavior (like `experimental.session.compacting`)
- **`tool`**: Register custom tools
- **`config`**: Modify configuration
- **`auth`**: Custom authentication

For the complete list of event types you can handle in the `event` function, see **[events.md](events.md)**.

### Example: Session Lifecycle

```javascript
export const LifecyclePlugin = async ({ $ }) => {
  return {
    event: async ({ event }) => {
      switch (event.type) {
        case "session.created":
          console.log("Session started:", event.properties.info?.id)
          break
        case "message.updated":
          if (event.properties.info?.role === "user") {
            console.log("User message received")
          }
          break
        case "session.idle":
          console.log("Session idle")
          break
      }
    },
  }
}
```

---

## Gastown Integration

For Gastown-specific integration patterns:

| Topic | Document |
|-------|----------|
| **Gastown Plugin** | `internal/opencode/plugin/gastown.js` |
| **Hook Mapping** | [design/gastown-plugin.md](../design/gastown-plugin.md) |
| **Integration Status** | [../archive/integration-summary.md](../archive/integration-summary.md) |

---

## Further Reading

| Topic | Document |
|-------|----------|
| **Configuration** | [configuration.md](configuration.md) |
| **Events Reference** | [events.md](events.md) |
| **External Resources** | [external-resources.md](external-resources.md) |
| **Quick Start** | [quickstart.md](quickstart.md) |
| **Runtime Comparison** | [../../agent-features.md](../../agent-features.md) |
