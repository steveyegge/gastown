# Session as First-Class Object

> Design for tracking agent sessions as first-class objects in Gas Town

## Overview

This document describes how to make "session" a first-class object with identity,
persistence, and queryability.

**Key principle: A session IS a Claude Code session.** The session boundary is defined
by Claude Code's own session lifecycle (the `CLAUDE_SESSION_ID`), not by Gas Town
constructs like molecules or spawn/handoff events. Molecules, beads, and steps are
work context that *enriches* the session record, but they don't define it.

This distinction matters because:
1. Claude Code sessions can exist without molecules (ad-hoc work, exploration)
2. A single molecule might span multiple Claude Code sessions (handoffs)
3. The authoritative session log is Claude Code's own telemetry, not our wrapper

## Background

### Current State

Sessions exist implicitly through several mechanisms:

1. **tmux sessions** - Named containers (e.g., `gt-gastown-Toast`) managed by `polecat/session_manager.go`
2. **Events log** - `session_start`/`session_end` events written to `~/gt/.events.jsonl`
3. **Claude session ID** - UUID from runtime, captured via `CLAUDE_SESSION_ID` env var
4. **Activity tracking** - Tmux-based timestamp tracking in `internal/activity/activity.go`

### Gaps (from research)

1. **No structured session logging** - Sessions aren't first-class objects with IDs tracked across lifecycle
2. **Limited step-level tracking** - Molecule steps don't log start/complete events automatically
3. **No session linking** - Handoffs don't preserve parent/child relationships
4. **Session identity fragmented** - Claude session ID vs tmux session name vs agent address

## Requirements

| Requirement | Description |
|-------------|-------------|
| **Identity** | Use Claude session ID as primary key, enrich with Gas Town metadata |
| **Boundaries** | Defined by Claude Code session lifecycle, not molecule boundaries |
| **Persistence** | Survive restarts; available for replay ("seance") |
| **Querying** | Find sessions by agent, time range, outcome, work unit |
| **Linking** | Track parent/child relationships across handoffs |
| **Tool Capture** | Record tool calls with arguments (privacy-filtered) and results |
| **Log Ingestion** | Ingest Claude Code's native conversation logs post-session |

## Data Model

### Session Object

```go
// Session represents a Claude Code session with Gas Town enrichment.
// The session ID IS the Claude session ID - Gas Town doesn't create its own.
type Session struct {
    // Identity (from Claude Code)
    ID          string    `json:"id"`           // Claude session ID (CLAUDE_SESSION_ID)
    TmuxSession string    `json:"tmux_session"` // e.g., "gt-gastown-Toast"
    Agent       string    `json:"agent"`        // e.g., "gastown/polecats/Toast"
    Rig         string    `json:"rig"`          // e.g., "gastown"
    Role        string    `json:"role"`         // e.g., "polecat"

    // Lifecycle
    StartedAt   time.Time `json:"started_at"`
    EndedAt     time.Time `json:"ended_at,omitempty"`
    Outcome     string    `json:"outcome,omitempty"` // "handoff", "done", "crash", "killed"

    // Work Context (Gas Town enrichment - optional)
    WorkUnit    string    `json:"work_unit,omitempty"`    // Hooked bead ID
    MoleculeID  string    `json:"molecule_id,omitempty"`  // Attached molecule
    StepRef     string    `json:"step_ref,omitempty"`     // Current step being executed

    // Linking
    ParentID    string    `json:"parent_id,omitempty"`    // Previous session in handoff chain
    ChildID     string    `json:"child_id,omitempty"`     // Next session after handoff
    ChainID     string    `json:"chain_id,omitempty"`     // Shared ID for entire work chain

    // Metadata
    CWD         string    `json:"cwd,omitempty"`          // Working directory
    Branch      string    `json:"branch,omitempty"`       // Git branch
    Commits     []string  `json:"commits,omitempty"`      // Commits made during session

    // Tool Activity (captured from Claude Code logs)
    ToolCalls   []ToolCall `json:"tool_calls,omitempty"` // Tool invocations during session
}

// ToolCall represents a single tool invocation with arguments.
// Arguments are captured with privacy controls (see Privacy section).
type ToolCall struct {
    Timestamp  time.Time         `json:"timestamp"`
    Tool       string            `json:"tool"`                  // e.g., "Bash", "Read", "Edit"
    Arguments  map[string]any    `json:"arguments,omitempty"`   // Tool-specific args (redacted per policy)
    Result     string            `json:"result,omitempty"`      // Truncated result summary
    DurationMs int64             `json:"duration_ms,omitempty"` // Execution time
    Success    bool              `json:"success"`               // Whether tool succeeded
}
```

### Session States

```
    ┌──────────┐
    │  spawn   │ ─────────────────────────────────────────────┐
    └────┬─────┘                                              │
         │                                                    │
         ▼                                                    │
    ┌──────────┐      ┌──────────┐      ┌──────────┐         │
    │  active  │ ───▶ │ handoff  │ ───▶ │  spawn   │ (new)   │
    └────┬─────┘      └──────────┘      └──────────┘         │
         │                                                    │
         ├───────────────────────────────────────────────────▶│
         │ done/crash/killed                                  │
         ▼                                                    │
    ┌──────────┐                                              │
    │ terminal │ ◀────────────────────────────────────────────┘
    └──────────┘
```

| State | Trigger | Outcome |
|-------|---------|---------|
| `active` | Session starts | Working |
| `handoff` | `gt handoff` | Spawns child session |
| `done` | `gt done` | Work complete, submit to MQ |
| `crash` | Tmux pane died | Respawn by Witness |
| `killed` | `gt polecat nuke` | Intentional termination |

## Storage Mechanism

### Design Choice: Events + Index

Sessions are tracked via two complementary mechanisms:

1. **Events log** (existing) - Append-only, audit trail
2. **Session index** (new) - Fast lookup, queryable state

This follows the event-sourcing pattern: events are the source of truth, index is derived.

### Session Index File

Location: `~/gt/.sessions/index.jsonl`

```jsonl
{"id":"abc-123","agent":"gastown/polecats/Toast","started_at":"2026-01-17T01:00:00Z","work_unit":"gt-xyz","chain_id":"chain-456"}
{"id":"def-789","agent":"gastown/polecats/Toast","started_at":"2026-01-17T02:00:00Z","parent_id":"abc-123","chain_id":"chain-456"}
```

### Individual Session Files (Optional)

For sessions with rich metadata (commits, step history), store detailed state:

Location: `~/gt/.sessions/<id>.json`

This enables:
- Fast index scans for listing/filtering
- Detailed lookup for specific sessions
- Easy archival of old sessions

### Why Not .beads/?

Session data is **operational state**, not **work items**:

| .beads/ | .sessions/ |
|---------|------------|
| Issues, molecules, work tracking | Runtime session state |
| Synced via `bd sync` | Local to machine |
| Cross-agent visible | Per-agent history |
| Semantic (bugs, tasks) | Observational (what happened) |

Sessions don't need git sync - they're local execution history.

## API Design

### Package: `internal/sessions`

```go
package sessions

// Manager handles session lifecycle and querying.
type Manager struct {
    townRoot string
}

// Start records a new session start.
// Returns the session ID (uses Claude session ID if available).
func (m *Manager) Start(opts StartOptions) (string, error)

// End records session termination.
func (m *Manager) End(sessionID string, outcome Outcome) error

// Handoff records a handoff and links parent/child.
// Returns the new session ID.
func (m *Manager) Handoff(parentID string, opts StartOptions) (string, error)

// Get retrieves a session by ID.
func (m *Manager) Get(sessionID string) (*Session, error)

// Query finds sessions matching criteria.
func (m *Manager) Query(q Query) ([]*Session, error)

// GetChain retrieves all sessions in a handoff chain.
func (m *Manager) GetChain(chainID string) ([]*Session, error)
```

### Query Interface

```go
type Query struct {
    Agent     string     // Filter by agent address
    Rig       string     // Filter by rig
    WorkUnit  string     // Filter by hooked bead
    ChainID   string     // Filter by handoff chain
    Since     time.Time  // Sessions after this time
    Until     time.Time  // Sessions before this time
    Outcome   string     // Filter by outcome (done, crash, etc.)
    Limit     int        // Max results
}
```

### CLI Commands

```bash
# List recent sessions
gt sessions list [--agent=<addr>] [--rig=<rig>] [--since=1h]

# Show session details
gt sessions show <session-id>

# Show handoff chain for a work unit
gt sessions chain <bead-id>

# Find sessions for an agent (seance prep)
gt sessions find --agent=gastown/polecats/Toast --since=24h
```

## Integration Points

### 1. Session Start (spawn)

**Location:** `polecat/session_manager.go:Start()`

```go
// After creating tmux session, register with sessions manager
sessionID, _ := sessions.Start(sessions.StartOptions{
    TmuxSession: sessionID,
    Agent:       address,
    Rig:         m.rig.Name,
    Role:        "polecat",
    WorkUnit:    opts.Issue,
    CWD:         workDir,
})
// Set GT_SESSION_UUID in tmux environment
m.tmux.SetEnvironment(sessionID, "GT_SESSION_UUID", sessionID)
```

### 2. Session End (gt done)

**Location:** `cmd/done.go`

```go
// Before exiting, record session end
sessions.End(os.Getenv("GT_SESSION_UUID"), sessions.OutcomeDone)
```

### 3. Handoff

**Location:** `cmd/handoff.go`

```go
// Record handoff with linking
parentID := os.Getenv("GT_SESSION_UUID")
newSessionID, _ := sessions.Handoff(parentID, sessions.StartOptions{
    // ... same opts as start
})
// The new session will pick up GT_SESSION_UUID from environment
```

### 4. Crash Detection

**Location:** `polecat/session_manager.go` (pane-died hook) or `witness/patrol.go`

```go
// On detecting crashed session
sessions.End(sessionID, sessions.OutcomeCrash)
// On respawn, link to crashed session
sessions.Start(sessions.StartOptions{
    ParentID: crashedSessionID,
    // ...
})
```

### 5. Events Integration

Session events are **emitted in addition to** index updates:

```go
func (m *Manager) Start(opts StartOptions) (string, error) {
    // ... create session record

    // Emit event for feed/audit
    events.LogFeed(events.TypeSessionStart, opts.Agent, events.SessionPayload(
        session.ID,
        opts.Agent,
        opts.WorkUnit,
        opts.CWD,
    ))

    return session.ID, nil
}
```

This maintains backward compatibility with existing event consumers.

## Chain ID Design

The `chain_id` links all sessions working on the same logical task:

```
Session 1 (chain: abc)  →  Session 2 (chain: abc)  →  Session 3 (chain: abc)
    │                          │                          │
    └── handoff ───────────────┴── handoff ───────────────┘
```

**Chain ID generation:**
- First session in chain: `chain_id = session_id`
- Subsequent sessions: inherit `chain_id` from parent

**Use cases:**
- "Show me all sessions that worked on gt-xyz"
- "What was the total cost of completing this issue?"
- "Replay the work from the beginning"

## Seance Integration

The existing seance concept (finding predecessor sessions for context) becomes trivial:

```go
// Old: grep through events, hope Claude session ID was logged
// New: direct query
sessions, _ := mgr.Query(sessions.Query{
    Agent: "gastown/polecats/Toast",
    Since: time.Now().Add(-24 * time.Hour),
})

// Or get the full chain for current work
chain, _ := mgr.GetChain(currentSession.ChainID)
```

## Migration Path

### Phase 1: Session identity and index

1. Create `internal/sessions` package
2. Use `CLAUDE_SESSION_ID` as primary session ID
3. Instrument session start/end in polecat manager
4. Add `gt sessions` CLI commands
5. Index file created automatically

### Phase 2: Claude Code log ingestion

1. Implement `gt session ingest` command
2. Parse Claude Code conversation files
3. Extract tool calls with privacy filtering
4. Store enriched session data

### Phase 3: Step-level enrichment

1. Emit `session_step_started`/`session_step_completed` events
2. Correlate molecule steps with Claude Code tool calls
3. Enable step-level replay

### Phase 4: Real-time streaming (optional)

1. Add Claude Code hooks for real-time tool call capture
2. Unix socket listener for live session events
3. Dashboard subscribes for live updates
4. Witness reacts to events without polling

## File Structure

```
~/gt/
├── .events.jsonl           # Existing events log (unchanged)
├── .sessions/              # NEW: Session state
│   ├── index.jsonl         # Session index (fast scan)
│   ├── <session-id>.json   # Detailed session state (optional)
│   └── archive/            # Old sessions (optional)
└── ...
```

## Backward Compatibility

- Existing `events.SessionPayload` unchanged
- `CLAUDE_SESSION_ID` environment variable still captured
- Events log continues to receive session events
- No changes to beads storage

## Cost/Benefit Analysis

| Cost | Benefit |
|------|---------|
| ~500 lines new code | First-class session identity |
| New index file (~KB per session) | Fast session queries |
| Instrumentation in 3-4 locations | Full handoff chain tracking |
| | Seance queries become trivial |
| | Foundation for step-level tracking |
| | Better debugging/observability |

## Tool Call Argument Privacy

Tool call arguments are valuable for debugging and replay, but may contain sensitive data.
Gas Town implements a tiered privacy model for argument capture.

### Privacy Tiers

| Tier | Description | Example Tools |
|------|-------------|---------------|
| `full` | All arguments captured verbatim | `Glob`, `Grep` (search patterns) |
| `redacted` | Sensitive values replaced with `[REDACTED]` | `Bash` (commands may contain secrets) |
| `metadata` | Only argument names/types, no values | `Edit`, `Write` (file contents) |
| `none` | Arguments not captured | Custom sensitive tools |

### Default Policies

```go
var DefaultPrivacyPolicy = map[string]PrivacyTier{
    // Full capture - search patterns are useful, rarely sensitive
    "Glob":      PrivacyFull,
    "Grep":      PrivacyFull,
    "Read":      PrivacyFull,  // File paths captured

    // Redacted - commands may contain env vars, secrets
    "Bash":      PrivacyRedacted,

    // Metadata only - file contents too large/sensitive
    "Edit":      PrivacyMetadata,
    "Write":     PrivacyMetadata,

    // WebFetch - URLs captured, response summarized
    "WebFetch":  PrivacyFull,
}
```

### Redaction Rules

For `redacted` tier tools:
1. Environment variable patterns: `$VAR`, `${VAR}` → `[ENV:VAR]`
2. Common secret patterns: `password=`, `token=`, `key=` → `[REDACTED]`
3. Base64-encoded strings >50 chars → `[BASE64:len]`
4. File paths in `/tmp`, home directories → preserved (useful for debugging)

### Configuration Override

Projects can customize privacy via `.gastown/privacy.yaml`:

```yaml
tool_privacy:
  Bash: none           # Never capture bash commands (high-security project)
  CustomTool: full     # Fully capture custom MCP tool
```

## Capturing Claude Code Logs

This is the core challenge: Claude Code's native session logs are the authoritative
record of what happened, but they're not directly exposed to Gas Town.

### Claude Code Log Architecture

Claude Code stores session data in:
```
~/.claude/
├── projects/
│   └── <project-hash>/
│       └── .claude/
│           └── conversations/       # Session conversations (JSON)
└── settings.json                    # Global settings
```

The conversation files contain:
- Full message history (user prompts, assistant responses)
- Tool calls with arguments and results
- Timestamps
- Token usage

### Capture Strategies

#### Strategy 1: Post-Session Log Ingestion (Recommended)

After a Claude Code session ends, ingest the conversation file:

```go
func IngestClaudeLog(sessionID string) (*SessionLog, error) {
    // 1. Find conversation file by session ID
    convPath := findConversationFile(sessionID)

    // 2. Parse JSON conversation
    conv, err := parseConversation(convPath)

    // 3. Extract tool calls with privacy filtering
    toolCalls := extractToolCalls(conv, privacyPolicy)

    // 4. Build session enrichment
    return &SessionLog{
        ToolCalls:   toolCalls,
        TokenUsage:  conv.Usage,
        MessageCount: len(conv.Messages),
    }, nil
}
```

**Pros:**
- Non-invasive (doesn't modify Claude Code behavior)
- Full access to conversation after completion
- Can apply privacy filtering before storage

**Cons:**
- Only available after session ends
- Depends on Claude Code's internal file format (may change)

#### Strategy 2: Real-Time Hook Streaming

Use Claude Code's hook system to stream tool calls as they happen:

```json
// .claude/settings.json
{
  "hooks": {
    "PostToolExecution": {
      "command": "gt session log-tool --tool=$TOOL --session=$CLAUDE_SESSION_ID"
    }
  }
}
```

**Pros:**
- Real-time visibility
- Works even if session crashes before completion
- Explicit integration point

**Cons:**
- Requires hook configuration
- Adds latency to each tool call
- Hook environment may not expose all arguments

#### Strategy 3: Conversation API Polling (Future)

If Claude Code exposes a local API for session state:

```go
// Hypothetical API
client := claude.NewLocalClient()
session, _ := client.GetSession(sessionID)
toolCalls := session.ToolCalls()
```

**Status:** Not currently available, but would be ideal.

### Recommended Implementation

**Phase 1: Post-session ingestion**
- Implement `gt session ingest` to pull Claude Code logs after `gt done`
- Apply privacy filtering during ingestion
- Store in session file

**Phase 2: Real-time hooks (optional)**
- Add hook-based streaming for real-time dashboards
- Supplement, don't replace, post-session ingestion

### Claude Code Log Format (Observed)

Based on inspection, conversation files use this structure:

```json
{
  "id": "conv-uuid",
  "messages": [
    {
      "role": "user",
      "content": "...",
      "timestamp": "2026-01-17T..."
    },
    {
      "role": "assistant",
      "content": "...",
      "tool_calls": [
        {
          "name": "Bash",
          "arguments": {"command": "git status"},
          "result": "...",
          "duration_ms": 150
        }
      ]
    }
  ],
  "usage": {
    "input_tokens": 1500,
    "output_tokens": 800
  }
}
```

**Note:** This format may change. The ingestion code should handle format evolution gracefully.

## Open Questions

1. **Session file retention** - How long to keep session files? Archive after N days?
2. **Cross-machine sessions** - If polecats run on multiple machines, need distributed index?
3. **Cost tracking integration** - Should session include token usage? Or separate concern?
4. **Claude Code format stability** - How to handle conversation file format changes?
5. **Privacy audit logging** - Should we log when redaction is applied for compliance?

## Related Documentation

- [Polecat Lifecycle](../concepts/polecat-lifecycle.md) - Session vs sandbox vs slot
- [Events](../../internal/events/events.go) - Event logging infrastructure
- [Architecture](architecture.md) - Overall Gas Town architecture
