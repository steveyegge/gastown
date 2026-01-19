---
# OpenCode Concept Reference
# Glossary of OpenCode concepts with links to all sources

last_updated: 2026-01-19
---

# OpenCode Concept Reference

A glossary of OpenCode concepts with links to official docs, source code, CLI, and SDK.

---

## How to Use This Document

For each concept, find:
- **What it is** - Brief description
- **Official Docs** - opencode.ai documentation
- **Source** - GitHub directories
- **CLI** - Command-line usage
- **SDK** - Programmatic API
- **Community** - Extensions and plugins

---

## Concept Glossary

### Sessions

The core unit of conversation and context in OpenCode.

| Source | Link |
|--------|------|
| **Official Docs** | [opencode.ai/docs/sessions](https://opencode.ai/docs/sessions/) |
| **Source** | [/packages/opencode/src/session](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/session) |
| **CLI** | `opencode --session <id>`, `opencode session list` |
| **SDK** | `client.session.create()`, `client.session.list()` |
| **HTTP API** | `GET /session`, `POST /session`, `GET /session/:id` |

**Key Operations**:
- Create: `opencode` or `POST /session`
- Resume: `opencode --session <id>` or `opencode -c` (last session)
- List: `opencode session list`
- Fork: `POST /session/:id/fork` with `{ messageID: "..." }`
- Export: `opencode export <id>`

---

### Plugins

Event-driven extensions that run within OpenCode.

| Source | Link |
|--------|------|
| **Official Docs** | [opencode.ai/docs/plugins](https://opencode.ai/docs/plugins/) |
| **Source** | [/packages/opencode/src/plugin](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin) |
| **Location** | `.opencode/plugin/<name>.js` |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |
| **Gastown Guide** | [plugin-guide.md](plugin-guide.md) |

**Key Events**:

| Event | Fires When |
|-------|------------|
| `session.created` | New session starts |
| `message.updated` | Response complete |
| `session.idle` | No activity (debounced) |
| `session.compacted` | Context compaction |
| `session.error` | Error occurs |

---

### Server / HTTP API

REST API for remote session management.

| Source | Link |
|--------|------|
| **Official Docs** | [opencode.ai/docs/server](https://opencode.ai/docs/server/) |
| **Source** | [/packages/server](https://github.com/anomalyco/opencode/tree/main/packages/server) |
| **CLI** | `opencode serve --port 4096` |
| **Default Port** | 4096 |

**Endpoints**:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/session` | List sessions |
| POST | `/session` | Create session |
| GET | `/session/:id` | Get session |
| POST | `/session/:id/message` | Send prompt |
| POST | `/session/:id/abort` | Cancel request |
| POST | `/session/:id/fork` | Fork from message |

**Environment Variables**:
- `OPENCODE_SERVER_PASSWORD` - HTTP basic auth
- `OPENCODE_SERVER_USERNAME` - Auth username (default: opencode)

---

### SDK (JavaScript/TypeScript)

Programmatic access to OpenCode.

| Source | Link |
|--------|------|
| **Official Docs** | [opencode.ai/docs/sdk](https://opencode.ai/docs/sdk/) |
| **Source** | [/packages/sdk](https://github.com/anomalyco/opencode/tree/main/packages/sdk) |
| **NPM** | [@opencode-ai/sdk](https://www.npmjs.com/package/@opencode-ai/sdk) |

**Key APIs**:
```javascript
import { createOpencode, createOpencodeClient } from "@opencode-ai/sdk"

// Start server + client
const { client, server } = await createOpencode({ port: 4096 })

// Connect to existing server
const client = createOpencodeClient({ baseUrl: "http://localhost:4096" })

// Session management
const session = await client.session.create({ body: { title: "My Session" } })
await client.session.prompt({
  path: { id: session.id },
  body: { parts: [{ type: "text", text: "Hello" }] }
})

// Context injection (no response)
await client.session.prompt({
  path: { id: session.id },
  body: { noReply: true, parts: [{ type: "text", text: "Context..." }] }
})
```

---

### SDK (Go)

Go SDK for programmatic orchestration.

| Source | Link |
|--------|------|
| **Repository** | [github.com/anomalyco/opencode-sdk-go](https://github.com/anomalyco/opencode-sdk-go) |
| **Gastown Plans** | [planning/stage2-sdk.md](../planning/stage2-sdk.md) |

---

### CLI

Command-line interface.

| Source | Link |
|--------|------|
| **Official Docs** | [opencode.ai/docs/cli](https://opencode.ai/docs/cli/) |
| **Source** | [/cmd](https://github.com/anomalyco/opencode/tree/main/cmd) |

**Execution Modes**:

| Mode | Command | Purpose |
|------|---------|---------|
| Interactive | `opencode` | TUI for humans |
| Attach | `opencode attach <url>` | Connect to server |
| Headless | `opencode run "prompt"` | Scripting |
| Server | `opencode serve` | API only |
| Web | `opencode web` | Browser UI |

**Key Flags**:

| Flag | Purpose |
|------|---------|
| `-c, --continue` | Resume last session |
| `-s, --session <id>` | Resume specific session |
| `--prompt` | Initial prompt |
| `--port` | Server port |
| `--agent` | Use specific agent |
| `-m, --model` | Override model |

---

### Tools / MCP

Model Context Protocol tools for extending capabilities.

| Source | Link |
|--------|------|
| **MCP Spec** | [modelcontextprotocol.io](https://modelcontextprotocol.io/) |
| **Source** | [/packages/opencode/src/tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

### Configuration

Configuration files and schema.

| Source | Link |
|--------|------|
| **Schema** | [opencode.ai/config.json](https://opencode.ai/config.json) |
| **Source** | [/packages/opencode/src/config](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/config) |

**Locations**:
- Project: `.opencode/config.json`
- Global: `~/.config/opencode/config.json`

**Environment**:
- `OPENCODE_CONFIG` - Path to config file
- `OPENCODE_PERMISSION` - Auto-approve permissions

---

### Agents

Multi-agent definitions.

| Source | Link |
|--------|------|
| **Source** | [/packages/opencode/src/agent](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/agent) |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

## Quick Lookup Table

| Concept | CLI | SDK | HTTP API | Docs | Source |
|---------|-----|-----|----------|------|--------|
| **Session** | `opencode --session` | `client.session.*` | `/session` | [docs](https://opencode.ai/docs/sessions/) | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/session) |
| **Plugin** | - | - | - | [docs](https://opencode.ai/docs/plugins/) | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin) |
| **Server** | `opencode serve` | `createOpencode()` | - | [docs](https://opencode.ai/docs/server/) | [src](https://github.com/anomalyco/opencode/tree/main/packages/server) |
| **Config** | - | - | - | - | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/config) |
| **Tools** | - | - | - | - | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) |
| **Agents** | `--agent` | - | - | - | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/agent) |

---

## External Resources

| Resource | URL | Notes |
|----------|-----|-------|
| **Repository** | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) | Source of truth |
| **Official Docs** | [opencode.ai/docs](https://opencode.ai/docs/) | May lag behind |
| **Awesome OpenCode** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) | Community extensions |
| **Ecosystem** | [opencode.ai/docs/ecosystem](https://opencode.ai/docs/ecosystem/) | Official directory |
| **ACP Spec** | [agentclientprotocol.com](https://agentclientprotocol.com/) | Protocol spec |
| **DeepWiki** | [deepwiki.com/anomalyco/opencode](https://deepwiki.com/anomalyco/opencode/) | Technical analysis |

---

## Key GitHub Issues

| Issue | Topic |
|-------|-------|
| [#7978](https://github.com/anomalyco/opencode/issues/7978) | ACP draft methods (session/fork) |
| [#1473](https://github.com/anomalyco/opencode/issues/1473) | Hooks support |
| [#2829](https://github.com/anomalyco/opencode/issues/2829) | Model variants |
