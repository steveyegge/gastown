# Convoy Lifecycle Design

> Making convoys actively converge on completion.

## Flow

```mermaid
flowchart TB
    %% ---- Creation ----
    create_manual(["gt convoy create"])
    create_sling(["gt sling<br/>(auto-convoy)"])
    create_formula(["gt formula run<br/>(convoy type)"])

    create_manual --> convoy["Convoy bead created<br/>(hq-cv-*)"]
    create_sling --> convoy
    create_formula --> convoy

    convoy --> track["Track issues<br/>via dep add --type=tracks"]

    %% ---- Dispatch ----
    track --> dispatch["gt sling dispatches<br/>work to polecats"]
    dispatch --> working["Polecats execute<br/>tracked issues"]

    %% ---- Completion detection ----
    working --> issue_close["Issue closes<br/>(gt done / bd close)"]

    issue_close --> check{"All tracked<br/>issues closed?"}

    check -->|No| feed["Feed next ready issue<br/>to available polecat"]
    feed --> working

    check -->|Yes| close["Close convoy +<br/>send notifications"]
    close --> done(["Convoy landed"])

    %% ---- Daemon observer (event-driven + stranded scan) ----
    issue_close -.-> obs_daemon["Daemon ConvoyManager<br/>Event poll (5s) + CheckConvoysForIssue<br/>Stranded scan (30s) as safety net"]

    obs_daemon -.-> check

    %% ---- Manual overrides ----
    force_close(["gt convoy close --force"]) -.-> close
    land(["gt convoy land<br/>(owned convoys)"]) -.-> close

    classDef successNode fill:#27ae60,color:#fff,stroke:none
    classDef extNode fill:#34495e,color:#ecf0f1,stroke:none

    class done successNode
    class obs_daemon extNode
```

Three creation paths feed into the same lifecycle. Completion is event-driven
via the daemon's `ConvoyManager`, which runs two goroutines:

- **Event poll** (every 5s): Polls all rig beads stores + hq via
  `GetAllEventsSince`, detects close events, and calls
  `convoy.CheckConvoysForIssue` — which both checks completion *and* feeds
  the next ready issue to a polecat.
- **Stranded scan** (every 30s): Runs `gt convoy stranded --json` to catch
  convoys missed by the event-driven path (e.g. after crash/restart). Feeds
  ready work or auto-closes empty convoys.

Manual overrides (`close --force`, `land`) bypass the check entirely.

> **History**: Witness and Refinery observers were originally planned as
> redundant observers but were removed (spec S-04, S-05). The daemon's
> multi-rig event poll + stranded scan provide sufficient coverage.

---

## Auto-convoy creation: what `gt sling` actually does

`gt sling` auto-creates a convoy for every bead it dispatches, unless
`--no-convoy` is passed. The behavior differs significantly between
single-bead and multi-bead (batch) sling.

### Single-bead sling

```
gt sling sh-task-1 gastown
```

1. Checks if `sh-task-1` is already tracked by an open convoy.
2. If not tracked: creates one auto-convoy `"Work: <issue-title>"` tracking
   that single bead.
3. Spawns one polecat, hooks the bead, starts working.

Result: 1 bead, 1 convoy, 1 polecat.

### Batch sling (3+ args, last arg is a rig)

```
gt sling sh-task-1 sh-task-2 sh-task-3 gastown
```

**Each bead gets its own separate auto-convoy.** The batch loop
(`sling_batch.go:71-241`) iterates each bead ID and runs the same
auto-convoy logic per bead. There is no grouping.

Result: 3 beads, **3 convoys**, 3 polecats — all dispatched in parallel
with 2-second delays between spawns.

There is no upper limit on the number of beads. `gt sling <10 beads> <rig>`
spawns 10 polecats with 10 individual convoys. The only throttle is
`--max-concurrent` (default 0 = unlimited).

### Why this matters

- **No dependency ordering.** All beads dispatch simultaneously. Even if
  `sh-task-2` has a `blocks` dep on `sh-task-1`, both get slung at the same
  time. The Phase 1 feeder's `isIssueBlocked` check only applies to
  daemon-driven convoy feeding (after a close event), not to the initial
  batch sling dispatch.
- **No convoy-level tracking.** Each convoy tracks exactly 1 bead. There is
  no single convoy that shows "3/3 tasks complete" for the batch. `gt convoy
  list` shows 3 independent convoys.
- **No sequential feeding.** Unlike a manual `gt convoy create "name" <beads>`
  followed by a single sling, batch sling does not use the convoy's
  `feedNextReadyIssue` path. All tasks start at once.

### How to get grouped convoy behavior

Create the convoy first, then sling individually (or let the daemon feed):

```
gt convoy create "Auth overhaul" sh-task-1 sh-task-2 sh-task-3
gt sling sh-task-1 gastown
# daemon auto-feeds sh-task-2 when sh-task-1 closes
# daemon auto-feeds sh-task-3 when sh-task-2 closes
# convoy auto-closes when all 3 are done
```

Or batch sling with `--no-convoy` if convoy tracking is not needed:

```
gt sling sh-task-1 sh-task-2 sh-task-3 gastown --no-convoy
```

---

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
3. **Manual close is inconsistent across docs**: `gt convoy close --force` exists, but some docs still describe it as missing
4. **Single observer**: No redundant completion detection
5. **Weak notification**: Convoy owner not always clear

## Design: Active Convoy Convergence

### Principle: Event-Driven, Centrally Managed

Convoy completion should be:
1. **Event-driven**: Triggered by issue close, not polling
2. **Centrally managed**: Single owner (daemon) avoids scattered side-effect hooks
3. **Manually overridable**: Humans can force-close

### Event-Driven Completion

When an issue closes, check if it's tracked by a convoy:

```mermaid
flowchart TD
    close["Issue closes"] --> tracked{"Tracked by convoy?"}
    tracked -->|No| done1(["Done"])
    tracked -->|Yes| check["Run gt convoy check"]
    check --> all{"All tracked<br/>issues closed?"}
    all -->|No| done2(["Done"])
    all -->|Yes| notify["Close convoy +<br/>send notifications"]
```

**Implementation**: The daemon's `ConvoyManager` event poll detects close events
via SDK `GetAllEventsSince` across all rig stores + hq. This catches all closes
regardless of source (CLI, witness, refinery, manual).

### Observer: Daemon ConvoyManager

The daemon's `ConvoyManager` is the sole convoy observer, running two
independent goroutines:

| Loop | Trigger | What it does |
|------|---------|--------------|
| **Event poll** | `GetAllEventsSince` every 5s (all rig stores + hq) | Detects close events, calls `CheckConvoysForIssue` |
| **Stranded scan** | `gt convoy stranded --json` every 30s | Feeds first ready issue via `gt sling`, auto-closes empty convoys |

Both loops are context-cancellable. The shared `CheckConvoysForIssue` function
is idempotent — closing an already-closed convoy is a no-op.

> **History**: The original design called for three redundant observers (Daemon,
> Witness, Refinery) per the "Redundant Monitoring Is Resilience" principle.
> Witness observers were removed (spec S-04) because convoy tracking is
> orthogonal to polecat lifecycle management. Refinery observers were removed
> (spec S-05) after S-17 found they were silently broken (wrong root path) with
> no visible impact, confirming single-observer coverage is sufficient.

### Issue-to-Rig Resolution

Convoys are rig-agnostic. A convoy like `hq-cv-6vjz2` lives in the hq store and
tracks issue IDs like `sh-pb6sa` — but it doesn't store which rig that issue
belongs to. The rig association is resolved at dispatch time via two lookups:

1. **Extract prefix**: `sh-pb6sa` → `sh-` (string parsing)
2. **Resolve rig**: look up `sh-` in `~/gt/.beads/routes.jsonl` → finds
   `{"prefix":"sh-","path":"gastown/.beads"}` → rig name is `gastown`

This happens in `feedFirstReady` (stranded scan path) and `feedNextReadyIssue`
(event poll path) just before calling `gt sling`. Issues with prefixes that
don't appear in `routes.jsonl` (or that map to `path="."` like `hq-*`) are
skipped — see `isSlingableBead()`.

Both paths check `isRigParked` after resolving the rig name. Issues targeting
parked rigs are logged and skipped rather than dispatched.

### Manual Close Command

`gt convoy close` is implemented, including `--force` for abandoned convoys.

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

### Current: `gt convoy close`

```bash
gt convoy close <convoy-id> [--reason=<reason>] [--notify=<agent>]
```

- Verifies tracked issues are complete by default
- `--force` closes even when tracked issues remain open
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

### Future: `gt convoy reopen`

```bash
gt convoy reopen <convoy-id>
```

Explicit reopen for clarity (currently implicit via add).

## Implementation Status

Core convoy manager is fully implemented and tested (see [spec.md](spec.md)
stories S-01 through S-18, all DONE). Remaining future work:

1. **P2: Owner field** - targeted notifications polish
2. **P3: Timeout/SLA** - deadline tracking

## Key Files

| Component | File |
|-----------|------|
| Convoy command | `internal/cmd/convoy.go` |
| Auto-convoy (sling) | `internal/cmd/sling_convoy.go` |
| Convoy operations | `internal/convoy/operations.go` (`CheckConvoysForIssue`, `feedNextReadyIssue`) |
| Daemon manager | `internal/daemon/convoy_manager.go` |
| Formula convoy | `internal/cmd/formula.go` (`executeConvoyFormula`) |

## Related

- [convoy.md](../../concepts/convoy.md) - Convoy concept and usage
- [watchdog-chain.md](../watchdog-chain.md) - Daemon/boot/deacon watchdog chain
- [mail-protocol.md](../mail-protocol.md) - Notification delivery
