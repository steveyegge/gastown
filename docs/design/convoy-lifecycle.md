# Convoy Lifecycle Design

> Making convoys actively converge on completion.

## Problem Statement

Convoys are passive trackers. They group work but don't drive it. The completion
loop has a structural gap:

```
Create → Assign → Execute → Issues close → ??? → Convoy closes
```

The `???` is "Deacon patrol runs `gt convoy check`" - a poll-based single point of
failure. When Deacon is down, convoys don't close. Work completes but the loop
never lands.

## Current State

### What Works
- Convoy creation and issue tracking
- `gt convoy status` shows progress
- `gt convoy stranded` finds unassigned work
- `gt convoy check` auto-closes completed convoys

### What Breaks
1. **Poll-based completion**: Only Deacon runs `gt convoy check`
2. **No event-driven trigger**: Issue close doesn't propagate to convoy
3. **No manual close**: Can't force-close abandoned convoys
4. **Single observer**: No redundant completion detection
5. **Weak notification**: Convoy owner not always clear

## Design: Active Convoy Convergence

### Principle: Event-Driven, Redundantly Observed

Convoy completion should be:
1. **Event-driven**: Triggered by issue close, not polling
2. **Redundantly observed**: Multiple agents can detect and close
3. **Manually overridable**: Humans can force-close

### Event-Driven Completion

When an issue closes, check if it's tracked by a convoy:

```
Issue closes
    ↓
Is issue tracked by convoy? ──(no)──► done
    │
   (yes)
    ↓
Run gt convoy check <convoy-id>
    ↓
All tracked issues closed? ──(no)──► done
    │
   (yes)
    ↓
Close convoy, send notifications
```

**Implementation options:**
1. Daemon hook on `bd update --status=closed`
2. Refinery step after successful merge
3. Witness step after verifying polecat completion

Option 1 is most reliable - catches all closes regardless of source.

### Redundant Observers

Per PRIMING.md: "Redundant Monitoring Is Resilience."

Three places should check convoy completion:

| Observer | When | Scope |
|----------|------|-------|
| **Daemon** | On any issue close | All convoys |
| **Witness** | After verifying polecat work | Rig's convoy work |
| **Deacon** | Periodic patrol | All convoys (backup) |

Any observer noticing completion triggers close. Idempotent - closing
an already-closed convoy is a no-op.

### Manual Close Command

**Desire path**: `gt convoy close` is expected but missing.

```bash
# Close a completed convoy
gt convoy close hq-cv-abc

# Force-close an abandoned convoy
gt convoy close hq-cv-xyz --reason="work done differently"

# Close with explicit notification
gt convoy close hq-cv-abc --notify mayor/
```

Use cases:
- Abandoned convoys no longer relevant
- Work completed outside tracked path
- Force-closing stuck convoys

### Convoy Owner/Requester

Track who requested the convoy for targeted notifications:

```bash
gt convoy create "Feature X" gt-abc --owner mayor/ --notify overseer
```

| Field | Purpose |
|-------|---------|
| `owner` | Who requested (gets completion notification) |
| `notify` | Additional subscribers |

If `owner` not specified, defaults to creator (from `created_by`).

### Convoy States

```
OPEN ──(all issues close)──► CLOSED
  │                             │
  │                             ▼
  │                    (add issues)
  │                             │
  └─────────────────────────────┘
         (auto-reopens)
```

Adding issues to closed convoy reopens automatically.

**New state for abandonment:**

```
OPEN ──► CLOSED (completed)
  │
  └────► ABANDONED (force-closed without completion)
```

### Timeout/SLA (Future)

Optional `due_at` field for convoy deadline:

```bash
gt convoy create "Sprint work" gt-abc --due="2026-01-15"
```

Overdue convoys surface in `gt convoy stranded --overdue`.

## Commands

### New: `gt convoy close`

```bash
gt convoy close <convoy-id> [--reason=<reason>] [--notify=<agent>]
```

- Closes convoy regardless of tracked issue status
- Sets `close_reason` field
- Sends notification to owner and subscribers
- Idempotent - closing closed convoy is no-op

### Enhanced: `gt convoy check`

```bash
# Check all convoys (current behavior)
gt convoy check

# Check specific convoy (new)
gt convoy check <convoy-id>

# Dry-run mode
gt convoy check --dry-run
```

### New: `gt convoy reopen`

```bash
gt convoy reopen <convoy-id>
```

Explicit reopen for clarity (currently implicit via add).

## Implementation Priority

1. **P0: `gt convoy close`** - Desire path, escape hatch
2. **P0: Event-driven check** - Daemon hook on issue close
3. **P1: Redundant observers** - Witness/Refinery integration
4. **P2: Owner field** - Targeted notifications
5. **P3: Timeout/SLA** - Deadline tracking

## Related

- [convoy.md](../concepts/convoy.md) - Convoy concept and usage
- [watchdog-chain.md](watchdog-chain.md) - Deacon patrol system
- [mail-protocol.md](mail-protocol.md) - Notification delivery
