# Opencode Integration Architecture

> Comprehensive guide to leveraging Opencode's full capabilities for Gastown integration
>
> **Purpose**: Define how Opencode's plugins, SDK, hooks, agents, and tools integrate with each Gastown concept
> **Status**: Architecture Design
> **Created**: 2026-01-16
> **Related**: [opencode-concept-analysis.md](opencode-concept-analysis.md), [opencode-orchestration.md](opencode-orchestration.md)

## Executive Summary

Based on comprehensive research of Opencode's architecture, this document defines how to best leverage Opencode's rich feature set for Gastown integration. Unlike the initial orchestration plan which focused on high-level abstractions, this document dives deep into Opencode-specific capabilities and how to extract core logic into reusable components with thin adapters for both Claude Code and Opencode.

**Key Opencode Capabilities**:
- **Plugin System**: Event hooks (`session.created`, `tool.execute.before/after`, etc.)
- **Custom Tools**: Define new tools agents can call (`.opencode/tool/`)
- **SDK/API**: HTTP REST API for programmatic session management
- **Agents & Subagents**: Primary agents and specialized subagents
- **Permissions**: Fine-grained tool access control
- **Server/Client Model**: Headless server with multiple client support

## Research Findings

### Opencode Architecture

**Core Components**:

1. **Server (`opencode serve`)**: Headless HTTP server exposing RESTful API
2. **Client Models**: CLI, TUI, desktop app, VS Code extension, SDK clients
3. **Session Management**: RESTful endpoints for creating, managing sessions
4. **Plugin System**: JavaScript/TypeScript plugins in `.opencode/plugin/`
5. **Custom Tools**: Project/global tools in `.opencode/tool/`
6. **Configuration**: `opencode.json` with layered inheritance (org → global → project)

**Event Hooks Available**:
```javascript
{
  event: async ({ event }) => {
    // event.type: "session.created", "session.idle", "message.updated", etc.
  },
  tool: {
    execute: {
      before: async (input, output) => { /* intercept tool calls */ },
      after: async (input, output) => { /* post-process results */ }
    }
  },
  stop: async () => { /* intercept session stop */ }
}
```

**Agent Configuration**:
```json
{
  "agent": {
    "custom-agent": {
      "mode": "primary" | "subagent",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are...",
      "steps": 15,
      "permission": {
        "bash": {"*": "ask", "git *": "allow", "rm *": "deny"},
        "edit": {"*": "deny", "src/*.ts": "allow"}
      }
    }
  }
}
```

### HTTP API Endpoints

```
POST   /session           # Create new session
GET    /session           # List sessions
GET    /session/:id       # Get session details
DELETE /session/:id       # Terminate session
POST   /session/:id/message  # Send message to session
GET    /events            # Server-Sent Events stream
```

---

## Integration Strategy: Core Logic Extraction

### Pattern: Core Logic + Thin Adapters

For each Gastown concept, we'll:
1. **Extract core logic** into runtime-agnostic modules
2. **Create thin adapters** for Claude Code and Opencode
3. **Use plugins/tools** to bridge Opencode to core logic

**Example Structure**:
```
internal/
├── mail/                    # Core mail logic (runtime-agnostic)
│   ├── delivery.go         # Mail delivery interface
│   └── injection.go        # Mail injection logic
├── claude/
│   └── mail_adapter.go     # Claude hook adapter (calls core)
└── opencode/
    └── mail_plugin.js      # Opencode plugin adapter (calls core)
```

---

## Concept-by-Concept Integration

### 1. Mayor 🎩

**Gastown Needs**:
- Spawn AI coordinator session
- Full context about workspace
- Coordinate work distribution

**Claude Integration**:
```go
// Current: Direct Claude spawning
claude.EnsureSettingsForRole(mayorDir, "mayor")
tmux.NewSession("gt-mayor", "claude --dangerously-skip-permissions", mayorDir)
```

**Opencode Integration Options**:

#### Option A: Plugin-Based (Recommended)
```javascript
// .opencode/plugin/gastown-mayor.js
export const GasTownMayor = async ({ client, $ }) => {
  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        // Initialize mayor context
        await $`gt prime --role mayor`.cwd(process.env.GT_MAYOR_DIR);
        await $`gt mail check --inject`;
      }
    }
  };
};
```

#### Option B: SDK-Based (Programmatic)
```go
// internal/opencode/mayor_spawner.go
func SpawnMayorViaSDK(mayorDir string) (sessionID string, err error) {
    client := opencode.NewClient("http://localhost:4096")
    
    // Create session with mayor configuration
    session, err := client.CreateSession(opencode.SessionConfig{
        Directory: mayorDir,
        Agent: "mayor",
        Prompt: "You are the Mayor, the primary AI coordinator...",
    })
    
    // Send initialization commands
    client.SendMessage(session.ID, "Run: gt prime --role mayor")
    
    return session.ID, nil
}
```

**Core Logic Extraction**:
```go
// internal/mayor/spawner.go (runtime-agnostic)
type MayorSpawner interface {
    Spawn(config MayorConfig) (SessionID, error)
    Initialize(sessionID SessionID) error
}

type MayorConfig struct {
    WorkDir     string
    RuntimeType string // "claude" | "opencode"
}

// internal/mayor/initializer.go (runtime-agnostic)
func InitializeMayor(workDir string) error {
    // Core initialization logic (works for any runtime)
    if err := mail.EnsureMailbox(workDir); err != nil {
        return err
    }
    if err := beads.EnsureHook(workDir, "mayor"); err != nil {
        return err
    }
    return nil
}
```

**Thin Adapters**:
```go
// internal/claude/mayor_adapter.go
type ClaudeMayorSpawner struct {
    tmux *tmux.Tmux
}

func (s *ClaudeMayorSpawner) Spawn(config mayor.MayorConfig) (string, error) {
    // Claude-specific spawning
    if err := claude.EnsureSettingsForRole(config.WorkDir, "mayor"); err != nil {
        return "", err
    }
    sessionID := "gt-mayor"
    cmd := "claude --dangerously-skip-permissions"
    return sessionID, s.tmux.NewSession(sessionID, cmd, config.WorkDir)
}

// internal/opencode/mayor_adapter.go
type OpencodeMayorSpawner struct {
    client *opencode.Client
}

func (s *OpencodeMayorSpawner) Spawn(config mayor.MayorConfig) (string, error) {
    // Opencode-specific spawning
    if err := opencode.EnsurePluginAt(config.WorkDir, ".opencode/plugin", "gastown-mayor.js"); err != nil {
        return "", err
    }
    session, err := s.client.CreateSession(opencode.SessionConfig{
        Directory: config.WorkDir,
        Agent: "mayor",
    })
    if err != nil {
        return "", err
    }
    return session.ID, nil
}
```

**Recommendation**: Use **Plugin-Based** for simplicity, **SDK-Based** for remote orchestration.

---

### 2. Polecat 🦨

**Gastown Needs**:
- Ephemeral worker agents
- Git worktree isolation
- Autonomous task execution

**Opencode Advantages**:
- Subagents for specialized tasks
- Custom tools for git worktree operations
- Fine-grained permissions per polecat

**Integration Approach**:

#### Custom Opencode Tool: Worktree Manager
```typescript
// .opencode/tool/worktree.ts
import { tool } from "@opencode-ai/plugin";

export const worktreeCreate = tool({
  description: "Create git worktree for polecat",
  args: {
    polecat: tool.schema.string().describe("Polecat name"),
    issue: tool.schema.string().describe("Issue ID")
  },
  async execute(args) {
    const { polecat, issue } = args;
    // Call core Gastown logic
    const result = await $`gt worktree create ${polecat} ${issue}`;
    return result.stdout;
  }
});

export const worktreeCleanup = tool({
  description: "Clean up polecat worktree",
  args: {
    polecat: tool.schema.string()
  },
  async execute(args) {
    await $`gt worktree cleanup ${args.polecat}`;
    return "Worktree cleaned up";
  }
});
```

#### Polecat-Specific Agent Configuration
```json
{
  "agent": {
    "polecat-default": {
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are a Polecat, an ephemeral worker agent...",
      "permission": {
        "bash": {
          "*": "ask",
          "git *": "allow",
          "make *": "allow",
          "go test *": "allow",
          "rm -rf *": "deny"
        },
        "edit": {"*": "allow"},
        "read": {"*": "allow"}
      }
    }
  }
}
```

#### Plugin: Polecat Lifecycle
```javascript
// .opencode/plugin/gastown-polecat.js
export const GasTownPolecat = async ({ $, directory, client }) => {
  const polecatName = process.env.GT_POLECAT_NAME;
  const issueID = process.env.GT_ISSUE_ID;
  
  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        // Initialize polecat context
        await $`gt prime --role polecat --polecat ${polecatName}`.cwd(directory);
        await $`gt mail check --inject`;
        
        // Load issue context
        if (issueID) {
          await $`gt issue load ${issueID}`.cwd(directory);
        }
      }
    },
    
    tool: {
      execute: {
        after: async (input, output) => {
          // Track polecat operations
          if (input.tool === "bash" && output.exitCode === 0) {
            await $`gt log polecat-operation --tool bash --success`.cwd(directory);
          }
        }
      }
    },
    
    stop: async () => {
      // Cleanup on session end
      await $`gt polecat cleanup ${polecatName}`.cwd(directory);
    }
  };
};
```

**Core Logic Extraction**:
```go
// internal/polecat/lifecycle.go (runtime-agnostic)
type PolecatLifecycle interface {
    Spawn(config PolecatConfig) (SessionID, error)
    Initialize(sessionID SessionID) error
    Cleanup(polecatName string) error
}

type PolecatConfig struct {
    Name        string
    IssueID     string
    WorktreeDir string
    RuntimeType string
}

// Shared initialization logic
func InitializePolecat(config PolecatConfig) error {
    // Create worktree
    if err := git.CreateWorktree(config.WorktreeDir, config.Name); err != nil {
        return err
    }
    
    // Set up mail
    if err := mail.EnsureMailbox(config.WorktreeDir); err != nil {
        return err
    }
    
    // Load issue context
    if config.IssueID != "" {
        if err := beads.LoadIssue(config.IssueID); err != nil {
            return err
        }
    }
    
    return nil
}
```

---

### 3. Mail 📧

**Gastown Needs**:
- Durable inter-agent messaging
- Mail injection on session start
- Work assignment delivery

**Integration Strategy**:

#### Core Mail Logic (Runtime-Agnostic)
```go
// internal/mail/delivery.go
type MailDelivery interface {
    SendMail(to, subject, body string) error
    CheckMail(recipient string) ([]Mail, error)
    InjectMail(sessionID string, mail []Mail) error
}

// Shared implementation
func CheckAndInjectMail(recipient, workDir string) ([]Mail, error) {
    // Query beads for mail
    mail, err := beads.QueryMail(recipient)
    if err != nil {
        return nil, err
    }
    
    // Format for injection
    formatted := FormatMailForInjection(mail)
    return formatted, nil
}
```

#### Claude Adapter (Hook-Based)
```json
// .claude/settings.json
{
  "hooks": {
    "SessionStart": {
      "command": "/bin/sh",
      "args": ["-lc", "gt prime && gt mail check --inject"]
    }
  }
}
```

#### Opencode Adapter (Plugin-Based)
```javascript
// .opencode/plugin/gastown-mail.js
export const GasTownMail = async ({ $, directory }) => {
  const role = process.env.GT_ROLE;
  const agentName = process.env.GT_AGENT_NAME;
  
  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        // Check and inject mail
        const result = await $`gt mail check --inject --role ${role}`.cwd(directory);
        console.log(`[gastown-mail] Injected mail: ${result.stdout}`);
      }
      
      // Periodic mail check for autonomous agents
      if (event?.type === "session.idle" && isAutonomous(role)) {
        await $`gt mail check --inject --quiet`.cwd(directory);
      }
    }
  };
};

function isAutonomous(role) {
  return ["polecat", "witness", "refinery", "deacon"].includes(role);
}
```

#### Opencode Custom Tool: Mail Operations
```typescript
// .opencode/tool/mail.ts
import { tool } from "@opencode-ai/plugin";

export const sendMail = tool({
  description: "Send mail to another Gastown agent",
  args: {
    to: tool.schema.string().describe("Recipient address (e.g., 'mayor/', 'gastown/crew/joe')"),
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

**Key Insight**: Mail injection is **already runtime-agnostic** via CLI command `gt mail check --inject`. Both Claude hooks and Opencode plugins call the same command!

---

### 4. Handoff 🤝 & Seance 🔮

**Gastown Needs**:
- Session forking/handoff
- Access to session history

**Challenge**: Claude-specific features (`--fork-session`, session history)

**Opencode Research Findings**:
- ✅ **Session Management**: Full CRUD via API
- ❓ **Session Forking**: Not documented (needs verification)
- ✅ **Session History**: Accessible via API (`GET /session/:id`)

**Integration Strategy**:

#### Option A: Opencode Native (If Supported)
```go
// internal/opencode/session_fork.go
func ForkSession(client *opencode.Client, parentSessionID string) (string, error) {
    // Check if Opencode supports session forking
    // (needs verification via EXP-007)
    return client.ForkSession(parentSessionID)
}
```

#### Option B: State Serialization (Fallback)
```go
// internal/handoff/state_serializer.go (runtime-agnostic)
type SessionState struct {
    ConversationHistory []Message
    ContextFiles        []string
    WorkUnits           []WorkUnit
    AgentState          map[string]interface{}
}

func SerializeSessionState(sessionID string) (*SessionState, error) {
    // Extract state from current session
    state := &SessionState{}
    
    // Query conversation history
    state.ConversationHistory = GetConversationHistory(sessionID)
    
    // Get active context
    state.ContextFiles = GetContextFiles()
    
    // Get work state
    state.WorkUnits = beads.GetActiveWork()
    
    return state, nil
}

func RestoreSessionState(state *SessionState, newSessionID string) error {
    // Restore state in new session
    for _, file := range state.ContextFiles {
        InjectContext(newSessionID, file)
    }
    
    for _, work := range state.WorkUnits {
        AssignWork(newSessionID, work)
    }
    
    return nil
}
```

#### Opencode Adapter: Handoff
```go
// internal/opencode/handoff_adapter.go
type OpencodeHandoffAdapter struct {
    client *opencode.Client
}

func (a *OpencodeHandoffAdapter) Handoff(fromSession, toSession string) error {
    // Serialize state from old session
    state, err := handoff.SerializeSessionState(fromSession)
    if err != nil {
        return err
    }
    
    // Create new session
    newSession, err := a.client.CreateSession(opencode.SessionConfig{
        Directory: state.WorkDir,
        Agent: state.AgentType,
    })
    if err != nil {
        return err
    }
    
    // Restore state
    return handoff.RestoreSessionState(state, newSession.ID)
}
```

#### Seance via Opencode API
```typescript
// .opencode/tool/seance.ts
import { tool } from "@opencode-ai/plugin";
import { createOpencodeClient } from "@opencode-ai/sdk";

export const seance = tool({
  description: "Query a previous agent session for context",
  args: {
    sessionID: tool.schema.string().describe("Session ID to query"),
    question: tool.schema.string().describe("Question to ask")
  },
  async execute(args) {
    const client = createOpencodeClient({ baseUrl: "http://localhost:4096" });
    
    // Get session history
    const session = await client.getSession(args.sessionID);
    const history = session.messages;
    
    // Search history for relevant context
    const relevantMessages = history.filter(msg => 
      msg.content.includes(args.question)
    );
    
    return {
      sessionID: args.sessionID,
      relevantContext: relevantMessages,
      summary: `Found ${relevantMessages.length} relevant messages`
    };
  }
});
```

---

### 5. Convoy 🚚 & Work Tracking

**Gastown Needs**:
- Track batched work across rigs
- Monitor agent progress
- Notify on completion

**Integration**: Already runtime-agnostic! Convoys use Beads for storage.

**Enhancement**: Opencode Tools for Convoy Operations
```typescript
// .opencode/tool/convoy.ts
import { tool } from "@opencode-ai/plugin";

export const convoyStatus = tool({
  description: "Check status of a Gastown convoy",
  args: {
    convoyID: tool.schema.string().describe("Convoy ID (e.g., hq-cv-abc)")
  },
  async execute(args) {
    const result = await $`gt convoy status ${args.convoyID} --json`;
    return JSON.parse(result.stdout);
  }
});

export const convoyCreate = tool({
  description: "Create a new Gastown convoy",
  args: {
    title: tool.schema.string(),
    issues: tool.schema.array(tool.schema.string())
  },
  async execute(args) {
    const issuesStr = args.issues.join(" ");
    const result = await $`gt convoy create "${args.title}" ${issuesStr}`;
    return result.stdout;
  }
});
```

**Plugin: Convoy Tracking**
```javascript
// .opencode/plugin/gastown-convoy.js
export const GasTownConvoy = async ({ $, client }) => {
  return {
    tool: {
      execute: {
        after: async (input, output) => {
          // Auto-track work in convoy
          if (input.tool === "bash" && output.args.command.includes("git commit")) {
            const convoyID = process.env.GT_CONVOY_ID;
            if (convoyID) {
              await $`gt convoy track ${convoyID} --commit ${output.result.hash}`;
            }
          }
        }
      }
    },
    
    event: async ({ event }) => {
      // Notify when convoy work completes
      if (event?.type === "session.idle") {
        const result = await $`gt convoy check-completion --json`;
        const convoys = JSON.parse(result.stdout);
        
        for (const convoy of convoys.completed) {
          await $`gt mail send --to mayor/ --subject "Convoy ${convoy.id} completed"`;
        }
      }
    }
  };
};
```

---

### 6. Nudge ⚡ & Real-Time Communication

**Gastown Needs**:
- Immediate agent-to-agent communication
- No persistent storage

**Current**: Tmux send-keys

**Opencode Enhancement**: SDK Event Streaming
```go
// internal/opencode/nudge_adapter.go
type OpencodeNudge struct {
    client *opencode.Client
}

func (n *OpencodeNudge) Nudge(targetSession, message string) error {
    // Option 1: Send via API
    return n.client.SendMessage(targetSession, message)
}

func (n *OpencodeNudge) BroadcastNudge(sessions []string, message string) error {
    // Broadcast to multiple sessions
    for _, sessionID := range sessions {
        if err := n.Nudge(sessionID, message); err != nil {
            // Log but continue
            log.Printf("Failed to nudge %s: %v", sessionID, err)
        }
    }
    return nil
}

func (n *OpencodeNudge) SubscribeToNudges(handler func(message string)) error {
    // Subscribe to real-time events
    stream, err := n.client.SubscribeEvents()
    if err != nil {
        return err
    }
    
    for event := range stream {
        if event.Type == "message.received" {
            handler(event.Content)
        }
    }
    
    return nil
}
```

**Both Approaches Work**:
- **Tmux**: Works for both Claude and Opencode (if running in tmux)
- **SDK**: Opencode-specific, works remotely

---

## Multi-Session Orchestration

### Remote Session Management

**Opencode Advantage**: HTTP API allows remote session orchestration

```go
// internal/orchestration/opencode/remote_orchestrator.go
type RemoteOpencodeOrchestrator struct {
    clients map[string]*opencode.Client  // host -> client
}

func (o *RemoteOpencodeOrchestrator) SpawnRemoteSession(host string, config SessionConfig) (SessionID, error) {
    client, ok := o.clients[host]
    if !ok {
        client = opencode.NewClient(fmt.Sprintf("http://%s:4096", host))
        o.clients[host] = client
    }
    
    session, err := client.CreateSession(opencode.SessionConfig{
        Directory: config.WorkDir,
        Agent: config.Role,
    })
    
    return session.ID, err
}

func (o *RemoteOpencodeOrchestrator) DistributeWork(work []WorkUnit, hosts []string) error {
    // Fan-out work across remote hosts
    for i, workUnit := range work {
        host := hosts[i % len(hosts)]
        sessionID, err := o.SpawnRemoteSession(host, SessionConfig{
            WorkDir: workUnit.WorkDir,
            Role: "polecat",
        })
        if err != nil {
            return err
        }
        
        // Assign work
        if err := o.AssignWork(sessionID, workUnit); err != nil {
            return err
        }
    }
    
    return nil
}
```

---

## Configuration Management

### Layered Configuration

**Opencode Configuration Hierarchy**:
1. Organization: `.well-known/opencode/opencode.json`
2. Global: `~/.config/opencode/opencode.json`
3. Project: `.opencode/opencode.json`

**Gastown Integration**:

#### Town-Level Opencode Config
```json
// ~/gt/settings/opencode.json
{
  "$schema": "https://opencode.ai/config.json",
  "theme": "gastown",
  "autoupdate": true,
  "model": "anthropic/claude-sonnet-4-5",
  
  "permission": {
    "bash": {"*": "ask", "git *": "allow", "gt *": "allow"},
    "edit": {"*": "allow"},
    "read": {"*": "allow"}
  },
  
  "agent": {
    "mayor": {
      "mode": "primary",
      "model": "anthropic/claude-opus-4",
      "prompt": "You are the Mayor, the primary AI coordinator...",
      "permission": {
        "bash": {"*": "allow"},
        "edit": {"*": "allow"}
      }
    },
    "deacon": {
      "mode": "primary",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are the Deacon, monitoring system health...",
      "permission": {
        "bash": {"*": "allow"},
        "edit": {"*": "deny"}
      }
    },
    "polecat": {
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4-5",
      "prompt": "You are a Polecat, an ephemeral worker...",
      "permission": {
        "bash": {"*": "ask", "git *": "allow", "make *": "allow"},
        "edit": {"*": "allow"}
      }
    }
  },
  
  "plugin": [
    ".opencode/plugin/gastown.js",
    ".opencode/plugin/gastown-mail.js",
    ".opencode/plugin/gastown-convoy.js"
  ]
}
```

#### Rig-Level Overrides
```json
// ~/gt/myrig/.opencode/opencode.json
{
  "agent": {
    "polecat": {
      "permission": {
        "bash": {"npm *": "allow", "yarn *": "allow"}
      }
    }
  }
}
```

---

## Implementation Roadmap

### Phase 1: Core Logic Extraction (Week 1)

**Goal**: Extract runtime-agnostic core logic

1. **Mail Delivery** (`internal/mail/`)
   - `delivery.go` - Core mail operations
   - `injection.go` - Mail injection logic
   - `formatting.go` - Mail formatting

2. **Session Lifecycle** (`internal/session/`)
   - `spawner.go` - Session spawning interface
   - `initializer.go` - Common initialization
   - `state.go` - Session state management

3. **Work Assignment** (`internal/work/`)
   - `assignment.go` - Work distribution
   - `tracking.go` - Progress tracking

**Deliverable**: Runtime-agnostic interfaces and implementations

---

### Phase 2: Opencode Adapters (Week 2)

**Goal**: Implement Opencode-specific adapters

1. **Plugin System**
   - Update `gastown.js` with richer hooks
   - Add `gastown-mail.js` for mail management
   - Add `gastown-convoy.js` for work tracking
   - Add `gastown-polecat.js` for worker lifecycle

2. **Custom Tools**
   - `mail.ts` - Mail operations
   - `convoy.ts` - Convoy management
   - `worktree.ts` - Git worktree operations
   - `seance.ts` - Session history queries

3. **SDK Integration**
   - `internal/opencode/client.go` - SDK wrapper
   - `internal/opencode/session_manager.go` - Session CRUD
   - `internal/opencode/remote_executor.go` - Remote orchestration

**Deliverable**: Full Opencode integration

---

### Phase 3: Unified Interface (Week 3)

**Goal**: Update agent managers to use unified interface

1. Update `internal/mayor/manager.go`
2. Update `internal/polecat/session_manager.go`
3. Update `internal/witness/manager.go`
4. Update `internal/refinery/manager.go`
5. Update `internal/deacon/manager.go`

**Pattern**:
```go
// Before
claude.EnsureSettingsForRole(workDir, role)
cmd := "claude --dangerously-skip-permissions"

// After
rc := config.LoadRuntimeConfig(workDir)
runtime.EnsureSettingsForRole(workDir, role, rc)
cmd := rc.BuildCommand()
```

**Deliverable**: All agents use runtime config

---

### Phase 4: Testing & Documentation (Week 4)

1. **Experimentation**
   - EXP-002: Plugin Installation
   - EXP-003: Work Assignment
   - EXP-007: Session Resume/Fork
   - EXP-008: Cross-Session Messaging

2. **Integration Tests**
   - Multi-session spawning
   - Mail delivery across runtimes
   - Convoy tracking
   - Remote orchestration

3. **Documentation**
   - Update user guide
   - Configuration examples
   - Migration guide
   - API reference

**Deliverable**: Production-ready, documented system

---

## Key Design Principles

### 1. Core Logic Extraction

**Principle**: Extract all business logic into runtime-agnostic modules

**Benefits**:
- Single source of truth
- Easier testing
- Reusable across runtimes
- Simpler maintenance

**Example**:
```go
// ✅ Good: Runtime-agnostic
func InitializeAgent(workDir, role string) error {
    if err := mail.EnsureMailbox(workDir); err != nil {
        return err
    }
    if err := beads.EnsureHook(workDir, role); err != nil {
        return err
    }
    return nil
}

// ❌ Bad: Runtime-coupled
func InitializeAgent(workDir, role string) error {
    if err := claude.EnsureSettings(workDir); err != nil {
        return err
    }
    // ... Claude-specific logic
}
```

### 2. Thin Adapters

**Principle**: Adapters should only handle runtime-specific I/O

**Responsibilities**:
- ✅ Spawn sessions
- ✅ Install hooks/plugins
- ✅ Format commands
- ❌ Business logic
- ❌ State management
- ❌ Work coordination

**Example**:
```go
// ✅ Good: Thin adapter
type ClaudeAdapter struct{}

func (a *ClaudeAdapter) SpawnSession(config SessionConfig) (string, error) {
    // Only handle Claude-specific spawning
    if err := claude.EnsureSettings(config.WorkDir); err != nil {
        return "", err
    }
    return tmux.NewSession(config.Name, "claude", config.WorkDir)
}

// Core logic called separately
func SpawnAgent(adapter RuntimeAdapter, config SessionConfig) error {
    sessionID, err := adapter.SpawnSession(config)
    if err != nil {
        return err
    }
    // Runtime-agnostic initialization
    return agent.Initialize(sessionID, config)
}
```

### 3. Plugin/Tool Composition

**Principle**: Use Opencode plugins/tools to call core Gastown logic

**Pattern**:
```javascript
// Opencode plugin calls Gastown CLI (which uses core logic)
export const GasTownPlugin = async ({ $ }) => ({
  event: async ({ event }) => {
    // Plugin is thin wrapper
    await $`gt prime`;  // Calls core logic
    await $`gt mail check --inject`;  // Calls core logic
  }
});
```

**Benefits**:
- Plugins stay simple
- Core logic in Go (tested, type-safe)
- Consistent behavior across runtimes

---

## Success Metrics

### Technical Metrics

- **Code Reuse**: >80% of logic is runtime-agnostic
- **Adapter Size**: <200 LOC per adapter
- **Test Coverage**: >90% for core modules
- **Performance**: <10% overhead vs native

### Functional Metrics

- **Feature Parity**: All core features work on both runtimes
- **Migration Time**: <1 hour to switch runtimes
- **Configuration**: Single source of truth per rig
- **Remote Sessions**: Works on 3+ hosts simultaneously

---

## Open Questions & Next Steps

### Questions Requiring Experimentation

1. **Opencode Session Forking** (EXP-007)
   - Does Opencode support session forking?
   - If not, how efficient is state serialization?

2. **Plugin Reliability** (EXP-002)
   - Do plugins load consistently?
   - What's the failure mode?

3. **Remote Performance** (EXP-006)
   - What's the latency for remote API calls?
   - How many concurrent remote sessions are practical?

4. **Event Hook Coverage** (EXP-010)
   - What events are available?
   - Are they sufficient for Gastown needs?

### Immediate Next Steps

1. **Research Validation**
   - Run EXP-007 (session resume/fork)
   - Run EXP-010 (event catalog)
   - Run EXP-012 (remote API)

2. **Prototype**
   - Extract mail delivery core logic
   - Create Opencode mail plugin
   - Test both Claude and Opencode

3. **Document**
   - Update concept analysis with Opencode specifics
   - Create migration guide
   - Write configuration examples

---

## Conclusion

Opencode provides a **rich, extensible platform** that aligns well with Gastown's architecture:

**Key Advantages**:
- ✅ **Plugin system** maps directly to Claude hooks
- ✅ **Custom tools** enable Gastown-specific operations
- ✅ **HTTP API** enables remote orchestration
- ✅ **Agent configuration** supports per-role customization
- ✅ **Permission system** provides fine-grained control

**Integration Strategy**:
1. **Extract core logic** into runtime-agnostic modules
2. **Create thin adapters** for Claude and Opencode
3. **Use plugins/tools** to bridge Opencode to core logic
4. **Leverage SDK** for remote orchestration

**Estimated Effort**: 3-4 weeks for full integration

**Bottom Line**: Opencode is a **natural fit** for Gastown. The plugin/tool system provides clean integration points, and the API enables advanced orchestration patterns that weren't possible with Claude Code alone.

---

**Last Updated**: 2026-01-16
**Owner**: Gastown Team
**Status**: Architecture Defined - Ready for Implementation
