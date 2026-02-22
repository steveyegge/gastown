# Scheduler Architecture

> Config-driven capacity-controlled polecat dispatch.

## Quick Start

Enable deferred dispatch and schedule some work:

```bash
# 1. Enable deferred dispatch (config-driven, no per-command flag)
gt config set scheduler.max_polecats 5

# 2. Schedule work via gt sling (auto-defers when max_polecats > 0)
gt sling gt-abc gastown              # Single task bead
gt sling gt-abc gt-def gt-ghi gastown  # Batch task beads
gt sling hq-cv-abc                   # Convoy (schedules all tracked issues)
gt sling gt-epic-123                 # Epic (schedules all children)

# 3. Check what's scheduled
gt scheduler status
gt scheduler list

# 4. Dispatch manually (or let the daemon do it)
gt scheduler run
gt scheduler run --dry-run    # Preview first
```

### Dispatch Modes

The `scheduler.max_polecats` config value controls dispatch behavior:

| Value | Mode | Behavior |
|-------|------|----------|
| `-1` (default) | Direct dispatch | `gt sling` dispatches immediately, near-zero overhead |
| `0` | Direct dispatch | Same as `-1` — `gt sling` dispatches immediately |
| `N > 0` | Deferred dispatch | `gt sling` labels/metadata only, daemon dispatches |

No per-invocation flag needed. The same `gt sling` command adapts automatically.

### Common CLI

| Command | Description |
|---------|-------------|
| `gt sling <bead> <rig>` | Sling bead (direct or deferred, per config) |
| `gt sling <bead>... <rig>` | Batch sling/schedule multiple beads |
| `gt sling <convoy-id>` | Sling/schedule all tracked issues in convoy |
| `gt sling <epic-id>` | Sling/schedule all children of epic |
| `gt scheduler status` | Show scheduler state and capacity |
| `gt scheduler list` | List all scheduled beads by rig |
| `gt scheduler run` | Trigger dispatch manually |
| `gt scheduler pause` | Pause all dispatch town-wide |
| `gt scheduler resume` | Resume dispatch |
| `gt scheduler clear` | Remove beads from scheduler |

### Minimal Example

```bash
gt config set scheduler.max_polecats 5
gt sling gt-abc gastown              # Defers: adds gt:queued label + metadata
gt scheduler status                  # "Queued: 1 total, 1 ready"
gt scheduler run                     # Dispatches -> spawns polecat -> strips metadata
```

---

## Overview

The scheduler solves **back-pressure** and **capacity control** for batched polecat dispatch.

Without the scheduler, slinging N beads spawns N polecats simultaneously, exhausting API rate limits, memory, and CPU. The scheduler introduces a governor: beads enter a waiting state and the daemon dispatches them incrementally, respecting a configurable concurrency cap.

The scheduler integrates into the daemon heartbeat as **step 14** — after all agent health checks, lifecycle processing, and branch pruning. This ensures the system is healthy before spawning new work.

```
Daemon heartbeat (every 3 min)
    |
    +- Steps 0-13: Health checks, agent recovery, cleanup
    |
    +- Step 14: gt scheduler run (capacity-controlled dispatch)
         |
         +- flock (exclusive)
         +- Check paused state
         +- Load config (max_polecats, batch_size)
         +- Count active polecats (tmux)
         +- Query ready scheduled beads (bd ready --label gt:queued)
         +- Dispatch loop (up to min(capacity, batch, ready))
         |    +- dispatchSingleBead -> executeSling
         +- Wake rig agents (witness, refinery)
         +- Save dispatch state
```

---

## Bead State Machine

A scheduled bead transitions through these states, tracked by labels and metadata:

```
                +----------------------------------------------+
                |                                              |
                v                                              |
          +----------+    dispatch ok     +--------------+    |
 schedule |          | -----------------> |              |    |
--------> |  QUEUED  |                    |  DISPATCHED  |    |
          |          |                    |              |    |
          +----------+                    +--------------+    |
                |                                              |
                +-- 3 failures --> +----------------+          |
                |                  | CIRCUIT-BROKEN |          |
                |                  +----------------+          |
                |                                              |
                +-- no metadata -> +--------------+            |
                |                  |  QUARANTINED |            |
                |                  +--------------+            |
                |                                              |
                +-- gt scheduler clear -> +-----------+        |
                                          | UNQUEUED  | ------+
                                          +-----------+  (re-schedulable)
```

### Label Transitions

| State | Label(s) | Metadata | Trigger |
|-------|----------|----------|---------|
| **QUEUED** | `gt:queued` | Present (delimiter block) | `scheduleBead()` |
| **DISPATCHED** | `gt:queue-dispatched` | Stripped | `dispatchSingleBead()` success |
| **CIRCUIT-BROKEN** | `gt:dispatch-failed` | Retained (failure count) | `dispatch_failures >= 3` |
| **QUARANTINED** | `gt:dispatch-failed` | Missing | Missing metadata at dispatch |
| **UNQUEUED** | (label removed) | Stripped | `gt scheduler clear` |

Key invariant: `gt:queued` is always removed on terminal transitions. Dispatched beads get `gt:queue-dispatched` as an audit trail so reopened beads aren't mistaken for actively scheduled ones.

---

## Entry Points

### CLI Entry Points

`gt sling` auto-detects the dispatch mode from config and the ID type:

| Command | Direct Mode (max_polecats=-1) | Deferred Mode (max_polecats>0) |
|---------|-------------------------------|-------------------------------|
| `gt sling <bead> <rig>` | Immediate dispatch | Schedule for later dispatch |
| `gt sling <bead>... <rig>` | Batch immediate dispatch | Batch schedule |
| `gt sling <epic-id>` | `runEpicSlingByID()` — dispatch all children | `runEpicScheduleByID()` — schedule all children |
| `gt sling <convoy-id>` | `runConvoySlingByID()` — dispatch all tracked | `runConvoyScheduleByID()` — schedule all tracked |

**Detection chain** in `runSling`:
1. `shouldDeferDispatch()` — check `scheduler.max_polecats` config
2. Batch (3+ args, last is rig) — `runBatchSchedule()` or `runBatchSling()`
3. `--on` flag set — formula-on-bead mode
4. 2 args + last is rig — `scheduleBead()` or inline dispatch
5. 1 arg, auto-detect type: epic/convoy/task

All schedule paths go through `scheduleBead()` in `internal/cmd/sling_schedule.go`.
All dispatch goes through `dispatchScheduledWork()` in `internal/cmd/capacity_dispatch.go`.

### Daemon Entry Point

The daemon calls `gt scheduler run` as a subprocess on each heartbeat (step 14):

```go
// internal/daemon/daemon.go
func (d *Daemon) dispatchScheduledWork() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    cmd := exec.CommandContext(ctx, "gt", "scheduler", "run")
    cmd.Env = append(os.Environ(), "GT_DAEMON=1", "BD_DOLT_AUTO_COMMIT=off")
    // ...
}
```

| Property | Value |
|----------|-------|
| Timeout | 5 minutes |
| Environment | `GT_DAEMON=1` (identifies daemon dispatch) |
| Gating | `scheduler.max_polecats > 0` (deferred mode) |

---

## Schedule Path

`scheduleBead()` performs these steps in order:

1. **Validate** bead exists, rig exists
2. **Cross-rig guard** — reject if bead prefix doesn't match target rig (unless `--force`)
3. **Idempotency** — skip if bead is already open with `gt:queued` label
4. **Status guard** — reject if bead is hooked/in_progress (unless `--force`)
5. **Validate formula** — verify formula exists (lightweight, no side effects)
6. **Cook formula** — `bd cook` to catch bad protos before daemon dispatch
7. **Build metadata** — `NewMetadata(rigName)` with all sling params
8. **Strip existing metadata** — ensure idempotent re-schedule (no duplicates)
9. **Write metadata** — `bd update --description=...` (inert without label)
10. **Add label** — `bd update --add-label=gt:queued` (atomic activation)
11. **Auto-convoy** — create convoy if not already tracked (unless `--no-convoy`)
12. **Log event** — feed event for dashboard visibility

**Metadata-before-label ordering** is critical: metadata without the label is inert (dispatch queries `bd ready --label gt:queued`, so unlabeled beads are invisible). The label is the atomic "commit." This prevents a race where dispatch fires between label-add and metadata-write, sees `meta==nil`, and irreversibly quarantines the bead.

**Rollback on failure**: if the label-add fails, the metadata write is rolled back to the original description.

---

## Dispatch Engine

`dispatchScheduledWork()` is the main dispatch loop:

```
flock(scheduler-dispatch.lock)
    |
    +- Load SchedulerState -> check paused?
    |
    +- Load SchedulerConfig (or defaults)
    |
    +- Check max_polecats > 0 (deferred mode only)
    |
    +- Determine limits:
    |    maxPolecats = config (or override)
    |    batchSize   = config (or override)
    |    spawnDelay  = config
    |
    +- Count active polecats (tmux session scan)
    |
    +- Compute capacity = maxPolecats - activePolecats
    |
    +- Query ready beads:
    |    bd ready --label gt:queued --json --limit=0
    |    (scans all rig DBs, deduplicates, skips circuit-broken)
    |
    +- PlanDispatch(maxPolecats, batchSize, activePolecats, readyBeads)
    |
    +- Dispatch loop:
    |    for each planned bead {
    |        dispatchSingleBead(bead, townRoot, actor)
    |        sleep(spawnDelay)   // between spawns
    |    }
    |
    +- Wake rig agents (witness, refinery) for each rig with dispatches
    |
    +- Save dispatch state (fresh read to avoid clobbering concurrent pause)
```

### dispatchSingleBead

Each bead dispatch:

1. **Parse metadata** from bead description
2. **Validate metadata** — quarantine immediately if missing (no circuit breaker waste)
3. **Reconstruct SlingParams** from metadata fields:
   - `FormulaName`, `Args`, `Vars`, `Merge`, `BaseBranch`, `Account`, `Agent`, etc.
   - `FormulaFailFatal=true` (rollback + requeue on failure)
   - `NoConvoy=true` (convoy already created at schedule time)
   - `NoBoot=true` (avoid lock contention in daemon dispatch loop)
   - `CallerContext="scheduler-dispatch"`
4. **Call `executeSling(params)`** — unified sling path (same as batch sling)
5. **On failure**: record failure in metadata, increment `dispatch_failures` counter
6. **On success**: strip scheduler metadata, swap `gt:queued` -> `gt:queue-dispatched`
7. **Log event** — feed event with polecat name

---

## Scheduler Metadata Format

Scheduler parameters are stored in the bead's description, delimited by a namespaced marker to avoid collision with user content.

### Delimiter

```
---gt:scheduler:v1---
```

Everything after the delimiter until the next delimiter (or end of description) is parsed as `key: value` lines.

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `target_rig` | string | Destination rig name |
| `formula` | string | Formula to apply at dispatch (e.g., `mol-polecat-work`) |
| `args` | string | Natural language instructions for executor |
| `var` | repeated | Formula variables, one `var: key=value` per line |
| `enqueued_at` | RFC3339 | Timestamp of schedule |
| `merge` | string | Merge strategy: `direct`, `mr`, `local` |
| `convoy` | string | Convoy bead ID (set after auto-convoy creation) |
| `base_branch` | string | Override base branch for polecat worktree |
| `no_merge` | bool | Skip merge queue on completion |
| `account` | string | Claude Code account handle |
| `agent` | string | Agent/runtime override |
| `hook_raw_bead` | bool | Hook without default formula |
| `owned` | bool | Caller-managed convoy lifecycle |
| `mode` | string | Execution mode: `ralph` (fresh context per step) |
| `dispatch_failures` | int | Consecutive failure count (circuit breaker) |
| `last_failure` | string | Most recent dispatch error message |

### Lifecycle

1. **Write** at schedule — `FormatMetadata()` appends block to description
2. **Read** at dispatch — `ParseMetadata()` extracts fields for `SlingParams`
3. **Strip** after dispatch — `StripMetadata()` removes the block on success

---

## Capacity Management

### Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `scheduler.max_polecats` | *int | `-1` | Max concurrent polecats (-1=direct, 0=disabled, N=deferred) |
| `scheduler.batch_size` | *int | `1` | Beads dispatched per heartbeat tick |
| `scheduler.spawn_delay` | string | `"0s"` | Delay between spawns (Dolt lock contention) |

Set via `gt config set`:

```bash
gt config set scheduler.max_polecats 5    # Enable deferred dispatch
gt config set scheduler.max_polecats -1   # Direct dispatch (default)
gt config set scheduler.batch_size 2
gt config set scheduler.spawn_delay 3s
```

### Dispatch Count Formula

```
toDispatch = min(capacity, batchSize, readyCount)

where:
  capacity   = maxPolecats - activePolecats
  batchSize  = scheduler.batch_size (default 1)
  readyCount = number of beads from bd ready --label gt:queued
```

### Active Polecat Counting

Active polecats are counted by scanning tmux sessions and matching role via `session.ParseSessionName()`. This counts **all** polecats (both scheduler-dispatched and directly-slung) because API rate limits, memory, and CPU are shared resources.

---

## Circuit Breaker

The circuit breaker prevents permanently-failing beads from causing infinite retry loops.

| Property | Value |
|----------|-------|
| Threshold | `maxDispatchFailures = 3` |
| Counter | `dispatch_failures` field in scheduler metadata |
| Break action | Add `gt:dispatch-failed` label, remove `gt:queued` |
| Reset | No automatic reset (manual intervention required) |

### Flow

```
Dispatch attempt fails
    |
    +- Increment dispatch_failures in metadata
    +- Store last_failure error message
    |
    +- dispatch_failures >= 3?
         +- Yes -> add gt:dispatch-failed, remove gt:queued
         |         (bead exits scheduler permanently)
         +- No  -> bead stays scheduled, retried next cycle
```

Beads without metadata are **quarantined immediately** (no circuit breaker retries) since they can never succeed:

```
dispatchSingleBead: meta == nil || meta.TargetRig == ""
    +- add gt:dispatch-failed, remove gt:queued (instant quarantine)
```

---

## Scheduler Control

### Pause / Resume

Pausing stops all dispatch town-wide. The state is stored in `.runtime/scheduler-state.json`.

```bash
gt scheduler pause    # Sets paused=true, records actor and timestamp
gt scheduler resume   # Clears paused state
```

Write is atomic (temp file + rename) to prevent corruption from concurrent writers.

### Clear

Removes beads from the scheduler by stripping the `gt:queued` label:

```bash
gt scheduler clear              # Remove ALL beads from scheduler
gt scheduler clear --bead gt-abc  # Remove specific bead
```

### Status / List

```bash
gt scheduler status         # Summary: paused, queued count, active polecats
gt scheduler status --json  # JSON output

gt scheduler list           # Beads grouped by target rig, with blocked indicator
gt scheduler list --json    # JSON output
```

`list` reconciles `bd list --label=gt:queued` (all queued) with `bd ready --label=gt:queued` (unblocked) to mark blocked beads.

---

## Scheduler and Convoy Integration

Convoys and the scheduler are complementary but distinct mechanisms. Convoys track completion of related beads; the scheduler controls dispatch capacity. Two paths exist for dispatching convoy work:

### Dispatch Paths

| Path | Trigger | Capacity Control | Use Case |
|------|---------|-----------------|----------|
| **Direct dispatch** | `gt sling <convoy-id>` (max_polecats=-1) | None (fires immediately) | Default mode — all issues dispatch at once |
| **Deferred dispatch** | `gt sling <convoy-id>` (max_polecats>0) | Yes (daemon heartbeat, max_polecats, batch_size) | Capacity-controlled — batched with back-pressure |

**Direct dispatch** (max_polecats=-1): `gt sling <convoy-id>` calls `runConvoySlingByID()` which dispatches all open tracked issues immediately via `executeSling()`. Each issue's rig is auto-resolved from its bead ID prefix. No capacity control — all issues dispatch at once.

**Deferred dispatch** (max_polecats>0): `gt sling <convoy-id>` calls `runConvoyScheduleByID()` which schedules all open tracked issues. The daemon dispatches incrementally via `gt scheduler run`, respecting `max_polecats` and `batch_size`. Use this for large batches where simultaneous dispatch would exhaust resources.

### When to Use Which

- **Small convoys (< 5 issues)**: Direct dispatch (default, max_polecats=-1)
- **Large batches (5+ issues)**: Set `scheduler.max_polecats` for capacity-controlled dispatch
- **Epics**: Same logic — `gt sling <epic-id>` auto-resolves mode from config

### Rig Resolution

`gt sling <convoy-id>` and `gt sling <epic-id>` auto-resolve the target rig per-bead from its ID prefix using `beads.ExtractPrefix()` + `beads.GetRigNameForPrefix()`. Town-root beads (`hq-*`) are skipped with a warning since they are coordination artifacts, not dispatchable work.

---

## Safety Properties

| Property | Mechanism |
|----------|-----------|
| **Schedule idempotency** | Skip if bead is open with `gt:queued` label |
| **Cross-rig guard** | Reject if bead prefix doesn't match target rig (unless `--force`) |
| **Dispatch serialization** | `flock(scheduler-dispatch.lock)` prevents double-dispatch |
| **Metadata-before-label** | Metadata is inert without label; label is atomic activation |
| **Post-dispatch label swap** | `gt:queued` -> `gt:queue-dispatched` prevents reopened beads from re-entering scheduler |
| **Formula pre-cooking** | `bd cook` at schedule time catches bad protos before daemon dispatch loop |
| **Rollback on label failure** | Metadata stripped if label-add fails (no orphaned metadata) |
| **Fresh state on save** | Dispatch re-reads state before saving to avoid clobbering concurrent pause |

---

## Code Layout

| Path | Purpose |
|------|---------|
| `internal/scheduler/capacity/config.go` | `SchedulerConfig` type, defaults, `IsDeferred()` |
| `internal/scheduler/capacity/metadata.go` | Metadata format/parse/strip |
| `internal/scheduler/capacity/pipeline.go` | `PlanDispatch()` pure function |
| `internal/cmd/sling.go` | CLI entry, config-driven routing |
| `internal/cmd/sling_schedule.go` | `scheduleBead()`, `shouldDeferDispatch()` |
| `internal/cmd/scheduler.go` | `gt scheduler` command tree |
| `internal/cmd/scheduler_epic.go` | Epic schedule/sling handlers |
| `internal/cmd/scheduler_convoy.go` | Convoy schedule/sling handlers |
| `internal/cmd/capacity_dispatch.go` | `dispatchScheduledWork()`, dispatch loop |
| `internal/daemon/daemon.go` | Heartbeat integration (`gt scheduler run`) |

---

## See Also

- [Watchdog Chain](watchdog-chain.md) — Daemon heartbeat, where scheduler dispatch runs as step 14
- [Convoys](../concepts/convoy.md) — Convoy tracking, auto-convoy on schedule
- [Operational State](operational-state.md) — Labels-as-state pattern used by scheduler labels
