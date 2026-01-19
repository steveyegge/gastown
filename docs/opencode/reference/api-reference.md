---
# OpenCode Concept Index
# Comprehensive glossary with links to all sources

last_updated: 2026-01-19
---

# OpenCode Concept Index

A comprehensive index of OpenCode concepts with links to official docs, source code, and this integration.

---

## Quick Lookup

| Concept | Official Docs | GitHub Source | Our Docs |
|---------|---------------|---------------|----------|
| [Sessions](#sessions) | [docs](https://opencode.ai/docs/sessions/) | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/session) | [plugin-guide](plugin-guide.md) |
| [Plugins](#plugins) | [docs](https://opencode.ai/docs/plugins/) | [src](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin) | [plugin-guide](plugin-guide.md) |
| [CLI](#cli) | [docs](https://opencode.ai/docs/cli/) | [cmd](https://github.com/anomalyco/opencode/tree/main/cmd) | - |
| [SDK (JS)](#sdk-javascript) | [docs](https://opencode.ai/docs/sdk/) | [sdk](https://github.com/anomalyco/opencode/tree/main/packages/sdk) | - |
| [SDK (Go)](#sdk-go) | - | [sdk-go](https://github.com/anomalyco/opencode-sdk-go) | [stage2 plans](../planning/phase2/sdk-plan.md) |
| [Server/API](#server--http-api) | [docs](https://opencode.ai/docs/server/) | [server](https://github.com/anomalyco/opencode/tree/main/packages/server) | - |
| [Tools/MCP](#tools--mcp) | [mcp](https://modelcontextprotocol.io/) | [tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) | - |
| [Config](#configuration) | [schema](https://opencode.ai/config.json) | [config](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/config) | - |
| [Agents](#agents) | - | [agent](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/agent) | - |
| [Authentication](#authentication) | - | - | - |
| [Models](#models) | - | [model](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/model) | - |
| [Logging](#logging) | - | - | - |
| [TUI](#tui) | - | [tui](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tui) | - |
| [Headless](#headless-mode) | - | - | - |
| [Custom Tools](#custom-tools) | - | - | - |
| [Third-Party](#third-party--community) | [ecosystem](https://opencode.ai/docs/ecosystem/) | - | - |

---

## Sessions

The core unit of conversation and context.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/sessions](https://opencode.ai/docs/sessions/) |
| **Source** | [/src/session](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/session) |

**CLI**:
```bash
opencode                      # Start new session
opencode --session <id>       # Resume session
opencode -c                   # Resume last session
opencode session list         # List sessions
opencode export <id>          # Export session
opencode import <file>        # Import session
```

**SDK**: `client.session.create()`, `client.session.list()`, `client.session.prompt()`

**HTTP API**: `GET /session`, `POST /session`, `GET /session/:id`, `POST /session/:id/fork`

---

## Plugins

Event-driven extensions that run within OpenCode.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/plugins](https://opencode.ai/docs/plugins/) |
| **Source** | [/src/plugin](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/plugin) |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |
| **Our Guide** | [plugin-guide.md](plugin-guide.md) |

**Location**: `.opencode/plugin/<name>.js`

**Events**:

| Event | When | Gastown Use |
|-------|------|-------------|
| `session.created` | Session starts | `gt prime`, `gt mail check` |
| `message.updated` | Response complete | Mail check (interactive) |
| `session.idle` | No activity | `gt costs record` |
| `session.compacted` | Context compaction | - |
| `session.error` | Error occurs | - |

---

## CLI

Command-line interface.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/cli](https://opencode.ai/docs/cli/) |
| **Source** | [/cmd](https://github.com/anomalyco/opencode/tree/main/cmd) |

**Modes**:

| Mode | Command | Purpose |
|------|---------|---------|
| Interactive | `opencode` | TUI for humans |
| Headless | `opencode run "prompt"` | Scripting |
| Server | `opencode serve` | API only |
| Attach | `opencode attach <url>` | Connect to server |
| Web | `opencode web` | Browser UI |

**Key Flags**: `-c` (continue), `-s` (session), `-m` (model), `--agent`, `--port`

---

## SDK (JavaScript)

Programmatic access via TypeScript/JavaScript.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/sdk](https://opencode.ai/docs/sdk/) |
| **Source** | [/packages/sdk](https://github.com/anomalyco/opencode/tree/main/packages/sdk) |
| **NPM** | [@opencode-ai/sdk](https://www.npmjs.com/package/@opencode-ai/sdk) |

```javascript
import { createOpencode, createOpencodeClient } from "@opencode-ai/sdk"
const { client } = await createOpencode({ port: 4096 })
const session = await client.session.create({ body: { title: "..." } })
await client.session.prompt({ path: { id: session.id }, body: { parts: [...] } })
```

---

## SDK (Go)

Go SDK for programmatic orchestration.

| Resource | Link |
|----------|------|
| **Repository** | [opencode-sdk-go](https://github.com/anomalyco/opencode-sdk-go) |
| **Our Plans** | [phase2/sdk-plan.md](../planning/phase2/sdk-plan.md) |

---

## Server / HTTP API

REST API for session management.

| Resource | Link |
|----------|------|
| **Official Docs** | [opencode.ai/docs/server](https://opencode.ai/docs/server/) |
| **Source** | [/packages/server](https://github.com/anomalyco/opencode/tree/main/packages/server) |

**Start**: `opencode serve --port 4096`

**Endpoints**:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/session` | List sessions |
| POST | `/session` | Create session |
| GET | `/session/:id` | Get session |
| POST | `/session/:id/message` | Send prompt |
| POST | `/session/:id/abort` | Cancel |
| POST | `/session/:id/fork` | Fork session |

**Auth**: `OPENCODE_SERVER_PASSWORD`, `OPENCODE_SERVER_USERNAME`

---

## Tools / MCP

Model Context Protocol tools.

| Resource | Link |
|----------|------|
| **MCP Spec** | [modelcontextprotocol.io](https://modelcontextprotocol.io/) |
| **Source** | [/src/tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

## Custom Tools

User-defined tools for extending capabilities.

| Resource | Link |
|----------|------|
| **Source** | [/src/tool](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tool) |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

## Configuration

Configuration files and schema.

| Resource | Link |
|----------|------|
| **Schema** | [opencode.ai/config.json](https://opencode.ai/config.json) |
| **Source** | [/src/config](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/config) |

**Locations**:
- Project: `.opencode/config.json`
- Global: `~/.config/opencode/config.json`

**Environment**:
- `OPENCODE_CONFIG` - Config file path
- `OPENCODE_PERMISSION` - Auto-approve permissions

---

## Agents

Multi-agent definitions and presets.

| Resource | Link |
|----------|------|
| **Source** | [/src/agent](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/agent) |
| **Community** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |

---

## Authentication

Authentication providers and configuration.

| Resource | Link |
|----------|------|
| **Source** | [/src/auth](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/auth) |

**Providers**: Various (GitHub Copilot, Anthropic, OpenAI, etc.)

---

## Models

Model configuration and routing.

| Resource | Link |
|----------|------|
| **Source** | [/src/model](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/model) |

**CLI**: `opencode models` (list available)

---

## TUI

Terminal User Interface.

| Resource | Link |
|----------|------|
| **Source** | [/src/tui](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/tui) |

---

## Headless Mode

Non-interactive execution for scripting.

**CLI**: `opencode run "prompt"` or `opencode run --attach <server>`

**Options**: `--format json`, `--session`, `--model`

---

## Logging

Logging and debugging.

| Resource | Link |
|----------|------|
| **Source** | [/src/log](https://github.com/anomalyco/opencode/tree/main/packages/opencode/src/log) |

---

## Third-Party & Community

Community extensions, tools, and integrations.

| Resource | Link |
|----------|------|
| **Awesome OpenCode** | [awesome-opencode](https://github.com/awesome-opencode/awesome-opencode) |
| **Ecosystem** | [opencode.ai/docs/ecosystem](https://opencode.ai/docs/ecosystem/) |
| **DeepWiki** | [deepwiki.com/anomalyco/opencode](https://deepwiki.com/anomalyco/opencode/) |

> **Tip**: Check community solutions before building something new.

---

## Key GitHub Issues

| Issue | Topic |
|-------|-------|
| [#7978](https://github.com/anomalyco/opencode/issues/7978) | ACP draft (session/fork) |
| [#1473](https://github.com/anomalyco/opencode/issues/1473) | Hooks support |
| [#2829](https://github.com/anomalyco/opencode/issues/2829) | Model variants |

---

## External Resources

| Resource | URL |
|----------|-----|
| **Repository** | [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) |
| **Official Docs** | [opencode.ai/docs](https://opencode.ai/docs/) |
| **ACP Spec** | [agentclientprotocol.com](https://agentclientprotocol.com/) |
| **Config Schema** | [opencode.ai/config.json](https://opencode.ai/config.json) |
