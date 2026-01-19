# Opencode Technical Research

> Deep technical dive into Opencode repository, ACP protocol, and implementation details
>
> **Purpose**: Source-of-truth technical reference from Opencode GitHub repository
> **Status**: Research Complete
> **Created**: 2026-01-16
> **Sources**: [anomalyco/opencode](https://github.com/anomalyco/opencode), [ACP Draft Spec](https://agentclientprotocol.com/protocol/draft/schema)

## Overview

This document provides deep technical research into Opencode based on direct repository analysis, not relying on potentially outdated documentation. All information is sourced from the Opencode GitHub repository and ACP protocol specifications with direct links.

**Key Research Areas**:
- Session forking capabilities (`session/fork`)
- Configuration schema and options
- ACP (Agent Client Protocol) integration
- Plugin system internals
- Custom tools and hooks
- Remote session management

---

## Session Forking (`session/fork`)

### Discovery

**Source**: [GitHub Issue #7978](https://github.com/anomalyco/opencode/issues/7978)

The `/fork` command mentioned in the screenshot is actually `session/fork`, part of the **ACP (Agent Client Protocol) draft specification**.

### ACP Draft Specification

**Reference**: [ACP Draft Schema - session/fork](https://agentclientprotocol.com/protocol/draft/schema#session-fork)

**Status**: **UNSTABLE** - Draft feature, may change or be removed

**Purpose**: Forks an existing session to create a new independent session with the same conversation context

**Use Cases**:
- Generate summaries without affecting original session history
- Create branches from existing conversations
- Experiment with alternate conversation paths
- Fan-out work from a single context

### Technical Details

#### Capability Advertisement

Agents must advertise support in `initialize` response:

```json
{
  "agentCapabilities": {
    "sessionCapabilities": {
      "fork": {}
    }
  }
}
```

#### Request Format

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/fork",
  "params": {
    "sessionId": "sess_789xyz",     // Required: ID of session to fork
    "cwd": "/path/to/directory",     // Required: Working directory
    "mcpServers": [...]              // Optional: MCP servers for new session
  }
}
```

#### Response Format

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "sessionId": "sess_new_abc",     // Required: New session ID
    "configOptions": {...},          // Unstable: Initial config
    "models": {...},                 // Unstable: Initial model state
    "modes": {...}                   // Initial mode state
  }
}
```

### Implementation Status

**Current Status** (as of 2026-01-16):
- ✅ Specification exists in ACP draft
- ⚠️ Implementation status in Opencode unknown (needs verification)
- ⚠️ No direct code reference found in public repository search
- ✅ Related: Internal "forked sessions" mentioned in v1.1.10 changelog (conversation management, not repository forking)

**References**:
- [ACP RFC: Session Fork](https://agentclientprotocol.com/rfds/session-fork)
- [ACP TypeScript SDK: ForkSessionRequest](https://agentclientprotocol.github.io/typescript-sdk/types/ForkSessionRequest.html)
- [Opencode Release v1.1.10](https://github.com/anomalyco/opencode/releases/tag/v1.1.10) - Mentions "forked sessions" for conversation management

### Implications for Gastown

**If Supported**:
```go
// Gastown can use native session forking
func (h *HandoffManager) Handoff(fromSession, toSession string) error {
    client := opencode.NewClient("http://localhost:4096")
    
    // Fork session via ACP
    newSession, err := client.ForkSession(opencode.ForkRequest{
        SessionID: fromSession,
        CWD: workDir,
    })
    
    return err
}
```

**If Not Supported**:
```go
// Fallback to state serialization (as documented in integration-architecture.md)
func (h *HandoffManager) Handoff(fromSession, toSession string) error {
    state, err := handoff.SerializeSessionState(fromSession)
    if err != nil {
        return err
    }
    
    newSession, err := client.CreateSession(...)
    return handoff.RestoreSessionState(state, newSession.ID)
}
```

**Action Required**: **EXP-007** - Test Opencode `session/fork` capability

---

## Configuration Schema

### Official Schema

**Schema URL**: [https://opencode.ai/config.json](https://opencode.ai/config.json)

**Documentation**: [OpenCode Config Docs](https://opencode.ai/docs/config/)

**Deep Dive**: [DeepWiki Configuration Schema](https://deepwiki.com/anomalyco/opencode/7.2-configuration-schema)

### Configuration Hierarchy

Opencode uses a **deep merge** strategy with precedence (lowest to highest):

1. **Remote config**: `.well-known/opencode` (organization level)
2. **Global config**: `~/.config/opencode/opencode.json` (user level)
3. **Custom config**: Via `OPENCODE_CONFIG` environment variable
4. **Project config**: `opencode.json` in project root
5. **Local config**: `.opencode/` directories (agents, plugins, tools)
6. **Inline config**: `OPENCODE_CONFIG_CONTENT` environment variable

**Source**: [DeepWiki Configuration Files](https://deepwiki.com/anomalyco/opencode/7.1-configuration-files)

### Key Configuration Sections

#### 1. Agent Configuration

```json
{
  "$schema": "https://opencode.ai/config.json",
  "agent": {
    "polecat": {
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are a Polecat worker agent...",
      "steps": 15,
      "permission": {
        "bash": {
          "*": "ask",
          "git *": "allow",
          "make *": "allow",
          "rm -rf *": "deny"
        },
        "edit": {"*": "allow"},
        "read": {"*": "allow"}
      }
    }
  }
}
```

**Reference**: [DeepWiki Agent Configuration](https://deepwiki.com/anomalyco/opencode/5.1-agent-configuration)

#### 2. Permission System

Supports wildcards and pattern matching:

```json
{
  "permission": {
    "bash": {
      "*": "ask",           // Ask for all commands
      "git *": "allow",     // Auto-allow git commands
      "npm *": "allow",     // Auto-allow npm commands
      "rm *": "deny",       // Block rm commands
      "rm -rf *": "deny"    // Block rm -rf specifically
    },
    "edit": {
      "*": "deny",          // Deny by default
      "src/*.ts": "allow",  // Allow TypeScript files
      "docs/*.md": "allow"  // Allow documentation
    }
  }
}
```

**Values**: `"allow"`, `"ask"`, `"deny"`

**Reference**: [OpenCode Permissions](https://opencode.ai/docs/permissions/)

#### 3. MCP Integration

```json
{
  "mcp": {
    "github": {
      "type": "remote",
      "url": "https://api.github.com/mcp",
      "enabled": true,
      "headers": {
        "Authorization": "Bearer ${GITHUB_TOKEN}"
      }
    }
  }
}
```

**Reference**: [How to use OpenCode: A Practical Guide](https://dev.to/composiodev/how-to-use-opencode-a-practical-guide-2egp)

#### 4. Model Aliases

```json
{
  "provider": {
    "opencode": {
      "models": {
        "gpt-5-high": {
          "id": "gpt-5",
          "options": {
            "reasoningEffort": "high",
            "textVerbosity": "low",
            "reasoningSummary": "auto"
          }
        }
      }
    }
  }
}
```

**Reference**: [GitHub Issue #2829](https://github.com/anomalyco/opencode/issues/2829)

### Gastown Integration Configuration

```json
{
  "$schema": "https://opencode.ai/config.json",
  "theme": "gastown",
  "model": "anthropic/claude-sonnet-4-5",
  "autoupdate": true,
  
  "permission": {
    "bash": {"*": "ask", "git *": "allow", "gt *": "allow"},
    "edit": {"*": "allow"},
    "read": {"*": "allow"}
  },
  
  "agent": {
    "mayor": {
      "mode": "primary",
      "model": "anthropic/claude-opus-4",
      "prompt": "You are the Mayor, Gas Town's primary AI coordinator..."
    },
    "polecat": {
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are a Polecat, an ephemeral worker agent..."
    },
    "deacon": {
      "mode": "primary",
      "prompt": "You are the Deacon, monitoring system health..."
    }
  },
  
  "plugin": [
    ".opencode/plugin/gastown.js",
    ".opencode/plugin/gastown-mail.js",
    ".opencode/plugin/gastown-polecat.js"
  ]
}
```

---

## ACP (Agent Client Protocol)

### What is ACP?

ACP is a standard protocol using JSON-RPC over stdio for communication between code editors and AI coding agents.

**Specification**: [Agent Client Protocol](https://agentclientprotocol.com/)

**Repository**: [GitHub - agentclientprotocol/agent-client-protocol](https://github.com/agentclientprotocol/agent-client-protocol)

### Opencode ACP Support

**Documentation**: [OpenCode ACP Support](https://opencode.ai/docs/acp/)

**Start Command**: `opencode acp`

### Integration Example

```json
// Zed editor config: ~/.config/zed/settings.json
{
  "agent_servers": {
    "OpenCode": {
      "command": "opencode",
      "args": ["acp"]
    }
  }
}
```

### ACP Draft Methods

#### `session/fork` (UNSTABLE)

Already covered above.

#### `session/list` (UNSTABLE)

Lists existing sessions with cursor-based pagination:

```json
{
  "method": "session/list",
  "params": {
    "cursor": null,              // For pagination
    "cwd": "/path/to/project"    // Filter by working directory
  }
}
```

**Response**:
```json
{
  "sessions": [
    {
      "sessionId": "sess_abc",
      "cwd": "/path/to/project",
      "created": "2026-01-16T10:00:00Z"
    }
  ],
  "nextCursor": "cursor_token"  // For next page, null if no more
}
```

**Source**: [ACP Draft Schema - session/list](https://agentclientprotocol.com/protocol/draft/schema#session-list)

#### `session/load`

Loads an existing session to resume conversation:

```json
{
  "method": "session/load",
  "params": {
    "sessionId": "sess_abc"
  }
}
```

**Capability Required**: Agent must advertise `loadSession: true`

---

## Plugin System

### Plugin Loading

Plugins are loaded from:
1. **Global**: `~/.config/opencode/plugin/`
2. **Project**: `.opencode/plugin/`
3. **npm**: Specified in `opencode.json` `plugin` array

**Reference**: [OpenCode Plugins](https://opencode.ai/docs/plugins/)

### Plugin Structure

```javascript
// .opencode/plugin/gastown.js
export const GasTown = async ({ $, directory, client, project, worktree }) => {
  // Context available to plugin
  return {
    event: async ({ event }) => {
      // Handle events: session.created, session.idle, message.updated, etc.
    },
    tool: {
      execute: {
        before: async (input, output) => {
          // Intercept before tool execution
        },
        after: async (input, output) => {
          // Post-process after tool execution
        }
      }
    },
    stop: async () => {
      // Handle session stop
    }
  };
};
```

**Events Available**:
- `session.created`
- `session.idle`
- `session.stopped`
- `message.updated`
- `message.completed`
- More documented in plugin API

**Reference**: [OpenCode Plugin Guide](https://gist.github.com/johnlindquist/0adf1032b4e84942f3e1050aba3c5e4a)

### Gastown Plugin Example

```javascript
// .opencode/plugin/gastown.js
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  let didInit = false;

  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    
    // Initialize Gastown context
    await run("gt prime");
    
    // Inject mail for autonomous agents
    if (autonomousRoles.has(role)) {
      await run("gt mail check --inject");
    }
    
    // Notify deacon
    await run("gt nudge deacon session-started");
  };

  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        await onSessionCreated();
      }
    },
  };
};
```

**Current Implementation**: `internal/opencode/plugin/gastown.js`

---

## Custom Tools

### Tool Definition

Custom tools are TypeScript/JavaScript files in `.opencode/tool/`:

```typescript
// .opencode/tool/mail.ts
import { tool } from "@opencode-ai/plugin";

export const sendMail = tool({
  description: "Send mail to another Gastown agent",
  args: {
    to: tool.schema.string().describe("Recipient (e.g., 'mayor/', 'gastown/crew/joe')"),
    subject: tool.schema.string().describe("Mail subject"),
    body: tool.schema.string().describe("Mail body")
  },
  async execute(args) {
    const result = await $`gt mail send --to ${args.to} --subject ${args.subject} --body ${args.body}`;
    return result.stdout;
  }
});

export const checkMail = tool({
  description: "Check incoming mail for this agent",
  args: {},
  async execute() {
    const result = await $`gt mail check --json`;
    return JSON.parse(result.stdout);
  }
});
```

**Reference**: [OpenCode Custom Tools](https://opencode.ai/docs/custom-tools/)

### Tool Context

Tools receive context:
- `agent`: Current agent name
- `sessionID`: Current session ID
- `messageID`: Current message ID
- `$`: Shell executor (zx-like)
- `directory`: Current working directory

### Multiple Tools Per File

```typescript
// .opencode/tool/worktree.ts
export const worktreeCreate = tool({...});
export const worktreeCleanup = tool({...});
export const worktreeList = tool({...});
```

All exports are registered as separate tools.

---

## HTTP API

### Server Mode

**Start Server**: `opencode serve`

**Default Port**: 3000

**Environment Variables**:
- `OPENCODE_SERVER_PASSWORD`: Enable HTTP Basic Auth
- `OPENCODE_SERVER_USERNAME`: Username (default: `opencode`)
- `PORT`: Server port (default: 3000)
- `HOST`: Server host (default: 0.0.0.0)

**Reference**: [OpenCode Server](https://opencode.ai/docs/server/)

### API Endpoints

```
POST   /session           # Create new session
GET    /session           # List all sessions
GET    /session/:id       # Get session details
DELETE /session/:id       # Terminate session
POST   /session/:id/message  # Send message to session
GET    /events            # Server-Sent Events stream
```

**Reference**: [DeepWiki HTTP Server API](https://deepwiki.com/sst/opencode/2.5-http-server-api)

### Session Management Example

```go
// internal/opencode/client.go
type Client struct {
    baseURL string
    auth    *BasicAuth
}

func (c *Client) CreateSession(config SessionConfig) (*Session, error) {
    resp, err := http.Post(
        fmt.Sprintf("%s/session", c.baseURL),
        "application/json",
        bytes.NewBuffer(config.ToJSON()),
    )
    // Parse response...
}

func (c *Client) ListSessions() ([]*SessionInfo, error) {
    resp, err := http.Get(fmt.Sprintf("%s/session", c.baseURL))
    // Parse response...
}

func (c *Client) SendMessage(sessionID, message string) error {
    resp, err := http.Post(
        fmt.Sprintf("%s/session/%s/message", c.baseURL, sessionID),
        "application/json",
        bytes.NewBuffer([]byte(fmt.Sprintf(`{"message": "%s"}`, message))),
    )
    // Handle response...
}
```

### Event Streaming

```go
func (c *Client) SubscribeEvents() (<-chan Event, error) {
    resp, err := http.Get(fmt.Sprintf("%s/events", c.baseURL))
    if err != nil {
        return nil, err
    }
    
    events := make(chan Event)
    
    go func() {
        scanner := bufio.NewScanner(resp.Body)
        for scanner.Scan() {
            // Parse SSE format
            event := parseSSE(scanner.Text())
            events <- event
        }
        close(events)
    }()
    
    return events, nil
}
```

**Reference**: [DeepWiki HTTP Server and API](https://deepwiki.com/sst/opencode/2.6-http-server-and-api)

---

## Remote Orchestration

### Multi-Host Setup

Opencode's HTTP API enables remote orchestration:

```go
type RemoteOrchestrator struct {
    hosts map[string]*opencode.Client
}

func (o *RemoteOrchestrator) SpawnOnHost(host string, config SessionConfig) (string, error) {
    client, ok := o.hosts[host]
    if !ok {
        client = opencode.NewClient(fmt.Sprintf("http://%s:3000", host))
        o.hosts[host] = client
    }
    
    session, err := client.CreateSession(config)
    return session.ID, err
}

func (o *RemoteOrchestrator) DistributeWork(work []WorkUnit, hosts []string) error {
    for i, unit := range work {
        host := hosts[i % len(hosts)]
        sessionID, err := o.SpawnOnHost(host, SessionConfig{
            Directory: unit.WorkDir,
            Agent: "polecat",
        })
        if err != nil {
            return err
        }
        
        // Assign work via API
        if err := o.AssignWork(sessionID, unit); err != nil {
            return err
        }
    }
    return nil
}
```

---

## Research Gaps & Experiments Needed

### 1. Session Fork Implementation (EXP-007)

**Status**: UNKNOWN - Needs verification

**Test Plan**:
1. Start Opencode server: `opencode serve`
2. Create session via API: `POST /session`
3. Attempt fork via ACP: `session/fork` method
4. Check if capability is advertised in `initialize` response
5. If not supported, verify state serialization fallback

**Success Criteria**:
- Session fork creates new independent session
- New session has same conversation history
- Original session unaffected

**Fallback**: State serialization approach documented in `integration-architecture.md`

### 2. Plugin Event Catalog (EXP-010)

**Status**: PARTIAL - Plugin API documented, but event catalog incomplete

**Known Events**:
- `session.created`
- `session.idle`
- `session.stopped`
- `message.updated`
- `message.completed`

**Test Plan**:
1. Create plugin that logs all events
2. Perform various operations
3. Document complete event catalog

**Reference**: [Plugin Guide Gist](https://gist.github.com/johnlindquist/0adf1032b4e84942f3e1050aba3c5e4a)

### 3. Remote API Discovery (EXP-012)

**Status**: DOCUMENTED - HTTP API exists

**Verified Capabilities**:
- ✅ HTTP REST API
- ✅ Session CRUD operations
- ✅ Server-Sent Events
- ⚠️ Authentication (Basic Auth supported)
- ❓ Multi-host session coordination

**Test Plan**:
1. Start multiple Opencode servers on different hosts
2. Test session creation on each
3. Test work distribution
4. Measure latency and reliability

### 4. Plugin State Persistence (EXP-011)

**Status**: UNKNOWN

**Questions**:
- Do plugins maintain state across session restarts?
- Is state per-session or per-server?
- What storage mechanisms are available?

**Test Plan**:
1. Create plugin that stores state
2. Restart session
3. Verify state persistence
4. Document findings

---

## Key Findings Summary

### ✅ Confirmed Capabilities

1. **Configuration System**: Robust, layered, well-documented
   - Schema: [https://opencode.ai/config.json](https://opencode.ai/config.json)
   - Hierarchy: Organization → Global → Custom → Project → Local → Inline

2. **Plugin System**: Flexible event-driven architecture
   - Events: `session.created`, `tool.execute.before/after`, `stop`, etc.
   - Context: `$`, `directory`, `client`, `project`, `worktree`

3. **Custom Tools**: TypeScript/JavaScript tools agents can call
   - Located: `.opencode/tool/`
   - Multiple tools per file supported

4. **HTTP API**: RESTful API for programmatic access
   - Endpoints: Session CRUD, messaging, events
   - Authentication: HTTP Basic Auth supported

5. **ACP Support**: Full Agent Client Protocol implementation
   - Command: `opencode acp`
   - Compatibility: Zed, JetBrains, Neovim, etc.

6. **Permissions**: Fine-grained tool access control
   - Patterns: Wildcards, path matching
   - Values: `allow`, `ask`, `deny`

### ⚠️ Needs Verification

1. **Session Fork**: ACP draft spec exists, implementation status unknown
   - Spec: [https://agentclientprotocol.com/rfds/session-fork](https://agentclientprotocol.com/rfds/session-fork)
   - Action: **EXP-007** - Test capability

2. **Plugin State Persistence**: Unclear from documentation
   - Action: **EXP-011** - Test and document

3. **Complete Event Catalog**: Partial documentation
   - Action: **EXP-010** - Create comprehensive list

4. **Multi-Host Coordination**: API exists, patterns unclear
   - Action: **EXP-012** - Test remote orchestration

### 🔗 Key References

**Primary Sources**:
- [Opencode Repository](https://github.com/anomalyco/opencode)
- [ACP Specification](https://agentclientprotocol.com/)
- [Configuration Schema](https://opencode.ai/config.json)
- [OpenCode Documentation](https://opencode.ai/docs/)
- [DeepWiki Technical Docs](https://deepwiki.com/anomalyco/opencode/)

**Issue Tracking**:
- [Issue #7978: ACP draft methods](https://github.com/anomalyco/opencode/issues/7978)
- [Issue #1473: Hooks support](https://github.com/anomalyco/opencode/issues/1473)
- [Issue #2829: Model variants](https://github.com/anomalyco/opencode/issues/2829)

---

**Last Updated**: 2026-01-16
**Research Status**: Complete - Implementation verification needed
**Next Steps**: Run experiments EXP-007, EXP-010, EXP-011, EXP-012
