---
# OpenCode Quick Reference
# Topic-based reference with links to official docs and source code

last_updated: 2026-01-19
---

# OpenCode Quick Reference

Quick reference for OpenCode concepts with links to official docs, source code, and community resources.

---

## External Sources

### Primary Sources

| Source | URL | Notes |
|--------|-----|-------|
| **OpenCode Repository** | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) | Source of truth - docs lag behind |
| **Official Documentation** | [opencode.ai/docs](https://opencode.ai/docs/) | May not reflect latest features |
| **ACP Specification** | [agentclientprotocol.com](https://agentclientprotocol.com/) | Agent Client Protocol draft |
| **Config Schema** | [opencode.ai/config.json](https://opencode.ai/config.json) | JSON schema for config |

### Community Resources

| Source | URL | Notes |
|--------|-----|-------|
| **Awesome OpenCode** | [github.com/awesome-opencode/awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) | Community plugins, tools, extensions |
| **Ecosystem** | [opencode.ai/docs/ecosystem](https://opencode.ai/docs/ecosystem/) | Official ecosystem directory |
| **DeepWiki** | [deepwiki.com/anomalyco/opencode](https://deepwiki.com/anomalyco/opencode/) | Technical deep dives |

### Key GitHub Issues

| Issue | Topic |
|-------|-------|
| [#7978](https://github.com/anomalyco/opencode/issues/7978) | ACP draft methods (session/fork, session/list) |
| [#1473](https://github.com/anomalyco/opencode/issues/1473) | Hooks support discussion |
| [#2829](https://github.com/anomalyco/opencode/issues/2829) | Model variants |

---

## Topic Reference

### CLI

Command-line interface for interactive and scripted use.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/cli](https://opencode.ai/docs/cli/) |
| **Source (cmd/)** | [github.com/anomalyco/opencode/tree/main/cmd](https://github.com/anomalyco/opencode/tree/main/cmd) |

**Execution Modes**:

| Mode | Command | Use Case |
|------|---------|----------|
| Interactive TUI | `opencode` | Human-in-the-loop |
| Attach to Server | `opencode attach <url>` | Connect to existing server |
| Headless | `opencode run "prompt"` | Scripting/automation |
| Server Only | `opencode serve` | API access |
| Web Interface | `opencode web` | Browser access |

**Key Flags**:
- `--continue/-c` - Resume last session
- `--session/-s <id>` - Resume specific session  
- `--prompt` - Initial prompt
- `--port` - Server port

---

### SDK

Programmatic access via JavaScript/TypeScript SDK.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/sdk](https://opencode.ai/docs/sdk/) |
| **NPM Package** | [@opencode-ai/sdk](https://www.npmjs.com/package/@opencode-ai/sdk) |
| **Source** | [github.com/anomalyco/opencode/tree/main/packages/sdk](https://github.com/anomalyco/opencode/tree/main/packages/sdk) |

**Key APIs**:
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
  body: { parts: [{ type: "text", text: "Your task..." }] }
})
```

---

### Go SDK

Go SDK for programmatic orchestration (Stage 2 plans).

| Resource | Link |
|----------|------|
| **Repository** | [github.com/anomalyco/opencode-sdk-go](https://github.com/anomalyco/opencode-sdk-go) |
| **Gastown Plans** | [stage2-sdk-orchestration.md](stage2-sdk-orchestration.md) |

---

### Server / HTTP API

REST API for session and message management.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/server](https://opencode.ai/docs/server/) |
| **Source (server/)** | [github.com/anomalyco/opencode/tree/main/packages/server](https://github.com/anomalyco/opencode/tree/main/packages/server) |

**Endpoints**:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/session` | GET | List sessions |
| `/session` | POST | Create session |
| `/session/:id` | GET | Get session info |
| `/session/:id/message` | POST | Send prompt |
| `/session/:id/abort` | POST | Cancel request |
| `/session/:id/fork` | POST | Fork session |

**Environment Variables**:
- `OPENCODE_SERVER_PASSWORD` - HTTP basic auth password
- `OPENCODE_SERVER_USERNAME` - HTTP basic auth username
- `OPENCODE_PERMISSION` - Auto-approve permissions

---

### Plugins

Event-driven extensions that run within OpenCode.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/plugins](https://opencode.ai/docs/plugins/) |
| **Source (plugin/)** | [github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin) |
| **Gastown Guide** | [plugin-guide.md](plugin-guide.md) |
| **Community Plugins** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

**Key Events (for Gastown)**:

| Event | Fires When | Gastown Use |
|-------|------------|-------------|
| `session.created` | New session starts | `gt prime`, `gt mail check` |
| `message.updated` | Response complete | Check mail for interactive roles |
| `session.idle` | No activity | `gt costs record` |
| `session.compacted` | Context compaction | Archive to `.beads` |

---

### Sessions

Session management, forking, and persistence.

| Resource | Link |
|----------|------|
| **Source (session/)** | [github.com/anomalyco/opencode/tree/main/packages/opencode/src/session](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/session) |
| **Fork via HTTP** | POST `/session/:id/fork` with `{ messageID: "..." }` |
| **Fork via TUI** | Type `/fork` in TUI |
| **ACP Fork RFC** | [agentclientprotocol.com/rfds/session-fork](https://agentclientprotocol.com/rfds/session-fork) |

---

### Tools / MCP

Model Context Protocol tools for extending agent capabilities.

| Resource | Link |
|----------|------|
| **MCP Spec** | [modelcontextprotocol.io](https://modelcontextprotocol.io/) |
| **Source (tools/)** | [github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) |
| **Community Tools** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

### Configuration

Configuration files and schema.

| Resource | Link |
|----------|------|
| **Config Schema** | [opencode.ai/config.json](https://opencode.ai/config.json) |
| **Source (config/)** | [github.com/anomalyco/opencode/tree/main/packages/opencode/src/config](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/config) |

**Config Locations**:
- Project: `.opencode/config.json`
- Global: `~/.config/opencode/config.json`

---

### Agents

Multi-agent capabilities and agent definitions.

| Resource | Link |
|----------|------|
| **Source (agent/)** | [github.com/anomalyco/opencode/tree/main/packages/opencode/src/agent](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/agent) |
| **Community Agents** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

## Gastown Integration Points

| Gastown Operation | OpenCode Approach |
|-------------------|-------------------|
| Start Mayor agent | `opencode --session <id>` in tmux |
| Send nudge | `tmux send-keys` OR SDK `session.prompt()` |
| Headless task | `opencode run "task"` |
| Polecat work | `opencode run "task"` |
| Check costs | Plugin `session.idle` → `gt costs record` |
| Session fork | HTTP POST `/session/:id/fork` |

---

## When to Check Community First

Before building a feature, check if it already exists:

1. **[Awesome OpenCode](https://github.com/awesome-opencode/awesome-opencode)** - Community extensions
2. **[Ecosystem](https://opencode.ai/docs/ecosystem/)** - Official directory
3. **[GitHub Issues](https://github.com/anomalyco/opencode/issues)** - Feature requests/discussions

OpenCode supports extension via SDK, plugins, MCP tools - consider using an existing solution over building from scratch.
