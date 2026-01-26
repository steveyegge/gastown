# Crash Recovery: Scrollback Context Restoration

## Summary

When the tmux server crashes or polecats die unexpectedly, we lose running Claude processes but retain valuable context in tmux scrollback buffers. This design proposes a `gt recover` command that captures scrollback history and injects it into newly spawned polecats, enabling them to resume their latest task with conversational context.

## Scope

**In scope:** Polecat recovery only. Polecats are the workhorses that hold task-specific context worth preserving.

**Out of scope:** Infrastructure agents (Mayor, Deacon, Witness, Refinery). These can be recovered through existing mechanisms (restart + bead state) since their context is more procedural than task-specific.

## Problem Statement

Currently, when a crash occurs:

1. **Tmux server exits** - All sessions and processes are lost
2. **Polecat restarts** - New Claude instance starts fresh
3. **Handoff bead read** - Agent gets structured state (task, files, status)
4. **Context gap** - Agent lacks the conversation history, reasoning, and partial work

The handoff bead system captures *what* the polecat was doing, but not *how* they were thinking about it or *where* they were in the conversation.

## Proposed Solution

### New Command: `gt recover`

```bash
gt recover                    # Interactive: list crashed polecat sessions
gt recover <session>          # Recover specific polecat
gt recover --all              # Recover all crashed polecats
gt recover --dry-run          # Show what would be recovered
```

### Context Limits

We want to capture the **latest task context**, not the entire session history. The limit is based on Claude's context window:

- **Target:** ~60% of 200k token context = ~120k tokens for recovery context
- **Rough conversion:** ~4 chars per token = ~480k characters = ~12k lines (at 40 chars/line avg)
- **Default limit:** 10,000 lines of scrollback (conservative estimate)

This leaves room for the new task prompt, tool outputs, and agent reasoning while capturing the most recent work.

### Recovery Flow

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Detect Crash   │────▶│ Capture Context  │────▶│ Spawn + Inject  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
        │                       │                        │
        ▼                       ▼                        ▼
  - Dead pane?            - tmux capture-pane      - Start Claude
  - Empty restored        - Parse scrollback       - Inject context
    session?              - Extract relevant       - Reference bead
  - Missing process?        portion                - Resume work
```

### Phase 1: Crash Detection

Identify recoverable sessions by checking:

```go
// RecoverableSession represents a session that can be recovered
type RecoverableSession struct {
    Name           string
    PaneID         string
    HasScrollback  bool
    ScrollbackSize int
    BeadPath       string
    LastActivity   time.Time
    CrashReason    string  // "server_exit", "process_died", "oom", etc.
}

func (t *Tmux) FindRecoverableSessions() ([]RecoverableSession, error) {
    // 1. Check for sessions with remain-on-exit panes (dead process)
    // 2. Check for restored sessions with no running process
    // 3. Check for sessions where pane command is shell but should be claude
}
```

### Phase 2: Context Capture

Extract and process scrollback:

```go
// CapturedContext holds the extracted conversation context
type CapturedContext struct {
    RawScrollback   string
    ParsedExchanges []Exchange
    LastUserMessage string
    LastAgentAction string
    TruncatedAt     int  // Line number if truncated
}

type Exchange struct {
    Role    string  // "user", "assistant", "system"
    Content string
    Tool    string  // If tool use was in progress
}

func CaptureContext(session string, maxLines int) (*CapturedContext, error) {
    // 1. tmux capture-pane -t session -p -S -maxLines
    // 2. Parse Claude Code output format to identify exchanges
    // 3. Identify last meaningful state
}
```

**Parsing considerations:**
- Claude Code has a specific output format with tool calls, thinking, etc.
- Need to identify user messages vs agent responses
- Detect partial tool executions (file writes, bash commands)
- Handle multi-turn context windows

### Phase 3: Context Injection

Start new Claude with recovery context:

```go
func RecoverSession(session string, ctx *CapturedContext, bead *Bead) error {
    // 1. Build recovery prompt
    prompt := buildRecoveryPrompt(ctx, bead)

    // 2. Start Claude in the session
    t.RespawnPane(pane, "claude")

    // 3. Wait for Claude to be ready
    t.WaitForRuntimeReady(session, claudeConfig, timeout)

    // 4. Inject recovery context
    t.NudgeSession(session, prompt)
}

func buildRecoveryPrompt(ctx *CapturedContext, bead *Bead) string {
    return fmt.Sprintf(`You were working on a task when your session crashed.

## Your Previous Conversation
%s

## Your Handoff State
%s

## Instructions
Resume from where you left off. If you were mid-task, complete it.
If you were waiting for user input, ask again.
Do not apologize for the crash - just continue working.`,
        ctx.FormatForInjection(),
        bead.Summary())
}
```

### Integration with Existing Systems

#### Handoff Beads

The recovery system complements beads:

| Beads Provide | Scrollback Provides |
|---------------|---------------------|
| Task assignment | Conversation flow |
| File list | Reasoning/thinking |
| Status flags | Partial work in progress |
| Structured data | User's exact words |

Recovery prompt should merge both:
```
[Bead state] + [Scrollback context] = Full recovery
```

#### Witness Integration

The Witness could detect crashes and trigger recovery:

```go
// In witness patrol loop
if session.IsDead() && session.HasScrollback() {
    log.Info("Detected crashed session with recoverable context")
    if err := recover.AutoRecover(session); err != nil {
        mail.Send(mayor, "Recovery failed for " + session.Name)
    }
}
```

#### Daemon Heartbeat

The daemon could maintain a "last known good" snapshot:

```go
// Every heartbeat, snapshot active sessions
func (d *Daemon) snapshotSessions() {
    for _, session := range activeSessions {
        scrollback := tmux.CapturePane(session, 1000)
        saveSnapshot(session, scrollback)
    }
}
```

This provides recovery even if tmux-resurrect wasn't triggered.

### Configuration

New config options in `gt.toml`:

```toml
[recovery]
enabled = true
auto_recover = false          # Auto-recover polecats on gt start?
max_context_lines = 10000     # ~120k tokens worth of scrollback
snapshot_interval = "5m"      # How often to snapshot (if enabled)
snapshot_dir = ".gt/snapshots"

[recovery.prompts]
# Customizable recovery prompt template
template = """
You crashed mid-task. Here's your recent conversation context:

{{.Scrollback}}

Resume from where you left off.
"""
```

### CLI UX

```
$ gt recover

Recoverable Sessions:
  1. gt-pixelsrc-witness    [crashed 5m ago]  2.3k lines context
  2. gt-reckoning-toast     [crashed 5m ago]  891 lines context
  3. hq-mayor               [crashed 5m ago]  1.2k lines context

Recover which session? [1-3, all, none]:
```

```
$ gt recover gt-pixelsrc-witness

Capturing context... 2,341 lines
Parsing exchanges... 12 user messages, 15 agent responses
Last activity: "Reading file src/auth/handler.go"
Handoff bead: pixelsrc/witness @ task-implement-oauth

Starting recovery...
✓ Claude started
✓ Context injected (2.1k tokens)
✓ Agent resuming work

Attach with: tmux attach -t gt-pixelsrc-witness
```

### Edge Cases

1. **No scrollback available** - Fall back to bead-only recovery
2. **Scrollback exceeds limit** - Truncate to last 10k lines (most recent context)
3. **Corrupted scrollback** - Skip parsing, inject raw text
4. **Multiple polecats crashed** - Recover each independently (no ordering required)
5. **Mid-tool-execution** - Detect and warn about partial file writes, incomplete commands

### Security Considerations

- Scrollback may contain sensitive data (API keys, passwords shown in output)
- Consider scrubbing known sensitive patterns before injection
- Snapshots should have restricted permissions (0600)
- Don't log full scrollback content

### Testing Strategy

1. **Unit tests** - Scrollback parsing, prompt building
2. **Integration tests** - Full crash/recover cycle in test tmux
3. **Chaos testing** - Random `kill -9` on agents, verify recovery

### Implementation Phases

**Phase 1: Basic Recovery (MVP)**
- `gt recover <session>` command
- Manual scrollback capture and injection
- Simple prompt template

**Phase 2: Smart Parsing**
- Parse Claude Code output format
- Extract structured exchanges
- Merge with bead state

**Phase 3: Auto-Recovery**
- Witness/daemon integration
- Automatic crash detection
- Background snapshots

**Phase 4: Polish**
- Recovery analytics/logging
- Configurable prompts
- Dry-run and preview modes

## Open Questions

1. Should we attempt to parse and extract just the "current task" portion of scrollback, or is a simple line limit sufficient?
2. Should recovery be opt-in or opt-out per polecat?
3. How do we detect that a polecat is in a bad state that will just crash again? (Defer for now - observe in practice first)

## Related

- [Handoff Beads](../concepts/beads.md)
- [Watchdog Chain](./watchdog-chain.md)
- [Operational State](./operational-state.md)
