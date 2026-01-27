# Gas Town Decision System Architecture

> Comprehensive technical documentation of the human-in-the-loop decision system.

## Executive Summary

The Gas Town Decision System provides structured human-in-the-loop gates for autonomous agents. When agents encounter architectural choices, ambiguous requirements, or risky operations, they create **decision beads** that block dependent work until a human (or authorized resolver) responds.

**Key Design Principles:**
1. **Decisions are beads** - Stored in the beads database with type `decision`, enabling dependency tracking
2. **Turn enforcement** - Agents must offer decisions before the Stop hook ends their turn
3. **Async-first** - Decisions can be resolved via CLI, TUI, or the injection system
4. **Fail-then-File integration** - Failure-related decisions validate that a "file bug" option exists

**Architecture Layers:**
```
┌─────────────────────────────────────────────────────────────┐
│                     Human Interface                          │
│   gt decision watch (TUI)  │  gt decision resolve (CLI)     │
├─────────────────────────────────────────────────────────────┤
│                     Command Layer                            │
│   internal/cmd/decision.go, decision_impl.go                │
├─────────────────────────────────────────────────────────────┤
│                     Hook Integration                         │
│   turn-mark, turn-check, turn-clear via Claude hooks        │
├─────────────────────────────────────────────────────────────┤
│                     Storage Layer                            │
│   internal/beads/beads_decision.go (DecisionFields)         │
├─────────────────────────────────────────────────────────────┤
│                     Notification Layer                       │
│   internal/inject/queue.go, internal/mail/router.go         │
└─────────────────────────────────────────────────────────────┘
```

---

## 1. Data Structures and Schema

### 1.1 DecisionFields (internal/beads/beads_decision.go:33-47)

The core decision payload embedded in a bead's description:

```go
type DecisionFields struct {
    Question    string           `json:"question"`              // The decision prompt
    Context     string           `json:"context,omitempty"`     // Additional context
    Options     []DecisionOption `json:"options"`               // 1-4 choices
    ChosenIndex int              `json:"chosen_index"`          // 0=pending, 1-indexed when resolved
    Rationale   string           `json:"rationale,omitempty"`   // Resolver's reasoning
    RequestedBy string           `json:"requested_by"`          // Agent address (e.g., "gastown/crew/max")
    RequestedAt string           `json:"requested_at"`          // RFC3339 timestamp
    ResolvedBy  string           `json:"resolved_by,omitempty"` // Who resolved (e.g., "human")
    ResolvedAt  string           `json:"resolved_at,omitempty"` // Resolution timestamp
    Urgency     string           `json:"urgency"`               // high, medium, low
    Blockers    []string         `json:"blockers,omitempty"`    // Bead IDs blocked by this decision
}
```

### 1.2 DecisionOption (internal/beads/beads_decision.go:26-31)

Individual choice within a decision:

```go
type DecisionOption struct {
    Label       string `json:"label"`                 // Short display name
    Description string `json:"description,omitempty"` // Detailed explanation
    Recommended bool   `json:"recommended,omitempty"` // Agent's recommendation
}
```

### 1.3 Issue Struct (internal/beads/beads.go:26-46)

Decisions are stored as standard beads with decision-specific labels:

```go
type Issue struct {
    ID          string   `json:"id"`           // e.g., "hq-abc123"
    Title       string   `json:"title"`        // Decision question (short form)
    Description string   `json:"description"`  // Full DecisionFields as markdown
    Status      string   `json:"status"`       // open, in_progress, closed
    Type        string   `json:"issue_type"`   // "decision"
    Labels      []string `json:"labels"`       // ["decision:pending", "urgency:high", "gt:decision"]
    CreatedAt   string   `json:"created_at"`
    CreatedBy   string   `json:"created_by"`
    Blocks      []string `json:"blocks"`       // Dependent beads blocked by this decision
    // ... standard bead fields
}
```

**Decision-specific labels:**
- `decision:pending` / `decision:resolved` - Current state
- `urgency:high` / `urgency:medium` / `urgency:low` - Priority level
- `gt:decision` - Type marker for queries

### 1.4 Turn Marker Files

File-based markers for turn enforcement:
- **Path:** `/tmp/.decision-offered-<session_id>`
- **Content:** Empty marker file
- **Lifecycle:** Created by `turn-mark`, checked by `turn-check`, cleared by `turn-clear`

### 1.5 Hook Input JSON

Structure passed to hooks via stdin:

```go
type turnHookInput struct {
    SessionID string `json:"session_id"`
    ToolInput struct {
        Command string `json:"command"`
    } `json:"tool_input"`
}
```

---

## 2. Claude Code Hook Integration

The decision system integrates with Claude Code via hooks defined in `.claude/settings.json`.

### 2.1 Hook Configuration (crew/.claude/settings.json)

```json
{
  "hooks": {
    "UserPromptSubmit": [{
      "matcher": "",
      "hooks": [{
        "type": "command",
        "command": "... && (bd decision check --inject || true) && (gt decision turn-clear || true)"
      }]
    }],
    "PostToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "type": "command",
        "command": "... && gt decision turn-mark"
      }]
    }],
    "Stop": [{
      "matcher": "",
      "hooks": [{
        "type": "command",
        "command": "... | gt decision turn-check"
      }]
    }]
  }
}
```

### 2.2 Hook Flow

```
┌─────────────────┐
│ User Prompt     │ → UserPromptSubmit hook
│ Submitted       │   → bd decision check --inject (inject pending decisions)
└────────┬────────┘   → gt decision turn-clear (clear last turn's marker)
         │
         ▼
┌─────────────────┐
│ Agent executes  │ → PostToolUse hook (Bash matcher)
│ Bash command    │   → gt decision turn-mark (marks if decision command)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Agent turn      │ → Stop hook
│ ends            │   → gt decision turn-check (validates decision was offered)
└─────────────────┘     Returns: {"decision":"block","reason":"..."} or {"decision":"proceed"}
```

### 2.3 Turn Enforcement Commands

| Command | Purpose | Hook |
|---------|---------|------|
| `gt decision turn-clear` | Clear marker from previous turn | UserPromptSubmit |
| `gt decision turn-mark` | Create marker if decision command ran | PostToolUse (Bash) |
| `gt decision turn-check` | Validate marker exists, block if missing | Stop |

**Detection logic** (`isDecisionCommand`):
- Matches `gt decision request` or `bd decision create`
- Case-sensitive
- Handles chained commands (`cmd1 && gt decision request`)

### 2.4 Injection Queue System (internal/inject/queue.go)

Asynchronous delivery to avoid API 400 errors from concurrent stdout writes:

```go
// Queue storage: .runtime/inject-queue/<session-id>.jsonl
type Entry struct {
    Type      EntryType `json:"type"`      // mail, decision, nudge
    Content   string    `json:"content"`   // Formatted message
    Timestamp int64     `json:"timestamp"`
}
```

**Flow:**
1. `bd decision check --inject` queues pending decisions
2. `gt inject drain` (PostToolUse hook) outputs queued content
3. File-based locking prevents concurrent access

---

## 3. Decision UI and Notification Systems

### 3.1 TUI Watch Interface (internal/tui/decision/)

Interactive terminal UI for monitoring and resolving decisions.

**Model State** (model.go:197-237):
```go
type Model struct {
    decisions      []DecisionItem  // Current pending decisions
    selected       int             // List cursor position
    selectedOption int             // 0=none, 1-4=option number
    inputMode      InputMode       // ModeNormal, ModeRationale, ModeText
    rationale      string          // User's rationale for choice
    filter         string          // "high", "all"
    peeking        bool            // Viewing agent terminal
    creatingCrew   bool            // Crew creation wizard active
}
```

**Key Bindings** (keys.go:41-124):
| Key | Action |
|-----|--------|
| j/k, ↑/↓ | Navigate decisions |
| 1-4 | Select option |
| Enter | Confirm selection |
| r | Add rationale |
| p | Peek agent terminal |
| d | Dismiss decision |
| c | Create new crew |
| ! | Filter high urgency |
| a | Show all |
| q | Quit |

**Polling:** Fetches decisions every 5 seconds via `gt decision list --json`.

### 3.2 CLI Commands (internal/cmd/decision.go)

| Command | Description |
|---------|-------------|
| `gt decision request` | Create a new decision |
| `gt decision list` | List pending decisions |
| `gt decision show <id>` | Show decision details |
| `gt decision resolve <id>` | Resolve with choice |
| `gt decision cancel <id>` | Cancel/dismiss decision |
| `gt decision dashboard` | Summary by urgency |
| `gt decision await <id>` | Block until resolved |
| `gt decision watch` | Launch TUI |

**Request flags:**
- `--prompt` / `--question`: Decision question
- `--option`: Option (repeatable, 1-4 times)
- `--context`: Additional context
- `--urgency`: high/medium/low (default: medium)
- `--blocks`: Bead IDs blocked by this decision
- `--parent`: Parent bead ID
- `--no-file-check`: Skip fail-then-file validation

### 3.3 Mail Notifications (internal/mail/router.go)

When a decision is created, notification mail is sent to the overseer:

```go
// formatDecisionMailBody creates markdown body with:
// - Decision ID
// - Urgency level
// - Question and context
// - Numbered options with recommendations marked
// - Blocking beads list
// - Command hint: gt decision resolve <id> --choice N
```

Resolution notifications are sent back to the requesting agent:
```go
// formatResolutionMailBody includes:
// - Decision ID
// - Chosen option label
// - Rationale (if provided)
// - Resolver identity
```

### 3.4 Tmux Notifications

Direct injection to agent sessions for immediate visibility:

```go
// addressToSessionIDs converts addresses to tmux session names
// e.g., "gastown/crew/max" → ["gt-gastown-crew-max"]
// Handles ambiguity between crew and polecats
```

---

## 4. Control Flow: End-to-End Trace

### 4.1 Decision Creation Flow

```
Agent: gt decision request --prompt "Which DB?" --option "PostgreSQL" --option "SQLite"
                │
                ▼
┌───────────────────────────────────────┐
│ decision_impl.go: runDecisionRequest  │
│ 1. Parse options, validate 1-4       │
│ 2. Build DecisionFields struct       │
│ 3. Apply fail-then-file validation   │
│ 4. Format as markdown description    │
│ 5. bd create --type=decision         │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│ beads_decision.go: CreateDecisionBead │
│ 1. Generate ID with hq- prefix       │
│ 2. Add labels: decision:pending,     │
│    urgency:X, gt:decision            │
│ 3. Set blocks relationships          │
│ 4. Store in town-level .beads/       │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│ Notification                          │
│ 1. Send mail to overseer             │
│ 2. PostToolUse: turn-mark creates    │
│    /tmp/.decision-offered-<session>  │
└───────────────────────────────────────┘
```

### 4.2 Decision Resolution Flow

```
Human: gt decision resolve hq-abc123 --choice 1 --rationale "Performance needs"
                │
                ▼
┌───────────────────────────────────────┐
│ decision_impl.go: runDecisionResolve  │
│ 1. Load decision bead                │
│ 2. Parse DecisionFields              │
│ 3. Update: chosen_index, rationale,  │
│    resolved_by, resolved_at          │
│ 4. Change label: decision:resolved   │
│ 5. Unblock dependent beads           │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│ Notification                          │
│ 1. Send resolution mail to requester │
│ 2. Nudge agent session if active     │
└───────────────────────────────────────┘
```

### 4.3 TUI Resolution Flow

```
┌─────────────────────────────────────────────────────────────┐
│ gt decision watch                                           │
│                                                             │
│ 1. fetchDecisions() → gt decision list --json              │
│ 2. Display in list with urgency indicators                 │
│ 3. User presses 1-4 to select option                       │
│ 4. User presses Enter to confirm                           │
│ 5. resolveDecision() → gt decision resolve <id> --choice N │
│ 6. Refresh list                                            │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Known Issues and Limitations

### 5.1 Settings Inheritance Bug (gt-x1r)

**Issue:** Claude Code doesn't inherit hooks from parent `.claude/settings.json`.

**Impact:** Crew workspaces without their own settings file don't fire Stop hooks, breaking turn enforcement.

**Workaround:** `gt crew add` must copy settings to each crew's `.claude/` directory.

### 5.2 Turn Marker Race Conditions

**Issue:** Multiple Stop hook firings in a single turn.

**Solution:** `turn-check` doesn't clear the marker (only `turn-clear` does), allowing multiple checks to pass.

### 5.3 Custom Text Not Implemented

The TUI shows "text" mode (key: t) but custom text responses are not yet supported. The mode displays a message explaining this limitation.

---

## 6. Configuration Files

### 6.1 Crew Hook Settings

**Location:** `{rig}/crew/.claude/settings.json`

Critical hooks for decision system:
- `UserPromptSubmit`: Inject pending decisions, clear turn marker
- `PostToolUse[Bash]`: Mark if decision command ran
- `Stop`: Validate decision was offered

### 6.2 Town-Level Beads

All decisions are stored in `{townRoot}/.beads/` (not rig-level) for cross-agent visibility.

---

## 7. Testing

Test coverage in `internal/cmd/decision_test.go`:

| Test | Coverage |
|------|----------|
| `TestFormatOptionsSummary` | Option formatting with recommendations |
| `TestUrgencyEmoji` | Urgency level display |
| `TestFormatDecisionMailBody` | Mail notification formatting |
| `TestTurnMarker*` | Turn enforcement marker lifecycle |
| `TestIsDecisionCommand` | Decision command detection |
| `TestTurnCheck*` | Strict/soft mode blocking |
| `TestHasFailureContext` | Fail-then-file keyword detection |
| `TestHasFileOption` | Fail-then-file option validation |

---

## 8. Future Considerations

1. **RPC layer** - Recent `--rpc` flag added to watch command for potential mobile/remote integration
2. **Text iteration** - Allow custom text responses for iteration without predefined options
3. **Batch resolution** - Resolve multiple decisions in single command
4. **Decision analytics** - Track resolution times, common patterns
5. **Automatic escalation** - Escalate unresolved high-urgency decisions after timeout

---

## Appendix A: File Reference

| File | Purpose |
|------|---------|
| `internal/cmd/decision.go` | Command definitions and flags |
| `internal/cmd/decision_impl.go` | Command implementations |
| `internal/cmd/decision_test.go` | Unit tests |
| `internal/beads/beads_decision.go` | DecisionFields, DecisionOption types |
| `internal/tui/decision/model.go` | TUI state and logic |
| `internal/tui/decision/view.go` | TUI rendering |
| `internal/tui/decision/keys.go` | Key bindings |
| `internal/inject/queue.go` | Injection queue system |
| `internal/mail/router.go` | Mail routing and notifications |
| `crew/.claude/settings.json` | Hook configuration |

---

*Generated by decisions_report crew member on 2026-01-27*
