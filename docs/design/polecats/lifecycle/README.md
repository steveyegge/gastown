# Polecat Lifecycle: Complete Technical Reference

> **Generated:** 2026-02-24
> **Scope:** Full investigation of polecat session creation, constraints, and edge cases

This document provides a comprehensive analysis of how polecat sessions are created,
configured, and managed throughout their lifecycle in Gas Town.

## Table of Contents

1. [Overview](#overview)
2. [Entry Points](#entry-points)
3. [Session Creation Flow](#session-creation-flow)
4. [Command & Environment Injection](#command--environment-injection)
5. [Constraints & Settings](#constraints--settings)
6. [Admission Control](#admission-control)
7. [Scheduling Logic](#scheduling-logic)
8. [Edge Cases & Failure Modes](#edge-cases--failure-modes)
9. [Key Files Reference](#key-files-reference)

## Supplementary Documents

| File | Coverage |
|------|----------|
| [dispatch-flow.md](dispatch-flow.md) | Visual diagrams, decision points, timing |
| [environment.md](environment.md) | Environment variables, configuration hierarchy |
| [edge-cases.md](edge-cases.md) | Zombie classes, race conditions, failure modes |
| [work-discovery.md](work-discovery.md) | gt prime --hook, work discovery, fallback nudges |
| [cleanup-flow.md](cleanup-flow.md) | gt done, refinery processing, MERGED signal, nuke |
| [orchestration.md](orchestration.md) | Formulas, hooks, daemons, patrols |

---

## Overview

Polecats are transient worker agents that execute discrete work items (beads) in isolated
git worktrees. Unlike crew workers (persistent, user-managed), polecats are:

- **Witness-managed**: Created and monitored by the rig's Witness agent
- **Transient**: Destroyed after work completion via `gt done`
- **Isolated**: Each polecat gets its own worktree and tmux session
- **Capacity-controlled**: Subject to `max_polecats` limits

### Lifecycle Summary

```
Dispatch Request (gt sling / convoy / scheduler)
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  ADMISSION CONTROL                                       │
│  • Parked rig check                                      │
│  • Capacity check (max_polecats)                         │
│  • Work bead readiness (no blockers)                     │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  RESOURCE ALLOCATION                                     │
│  • Allocate polecat name (or find idle)                  │
│  • Create/repair worktree                                │
│  • Hook bead to polecat                                  │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  SESSION CREATION                                        │
│  • Build startup command with environment vars           │
│  • Create tmux session with beacon prompt                │
│  • Set session environment variables                     │
│  • Wait for Claude ready + send nudges                   │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  WORK EXECUTION (Polecat runs autonomously)              │
│  • gt prime --hook loads context                         │
│  • Execute work on hooked bead                           │
│  • gt done → MR creation → cleanup                       │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  CLEANUP                                                 │
│  • Witness monitors for completion                       │
│  • Refinery processes MR                                 │
│  • Worktree nuked after merge                            │
└─────────────────────────────────────────────────────────┘
```

---

## Entry Points

### 1. `gt sling` Command (Primary)

**File:** `internal/cmd/sling.go:161-249`
**Function:** `runSling()`

The most common entry point for polecat creation. When target is a rig name:

```bash
gt sling gt-abc gastown    # Dispatch bead gt-abc to gastown rig
```

**Flow:**
1. Validate target → `resolveTarget()` in `sling_target.go`
2. Check if target is rig name → `IsRigName()`
3. Check parked status → `IsRigParked()`
4. Execute dispatch → `executeSling()` in `sling_dispatch.go`
5. Spawn polecat → `SpawnPolecatForSling()` in `polecat_spawn.go`

### 2. Batch Sling

**File:** `internal/cmd/sling_batch.go`

Dispatches multiple beads to a single rig:

```bash
gt sling gt-abc gt-def gt-ghi gastown
```

Calls `executeSling()` for each bead, spawning one polecat per bead.

### 3. Capacity Scheduler (Daemon)

**File:** `internal/cmd/capacity_dispatch.go:26-200`
**Function:** `dispatchScheduledWork()`

The daemon's heartbeat-driven dispatch mechanism:

1. Runs every 3 minutes (default)
2. Queries pending sling context beads from HQ
3. Filters by readiness (no blockers) and capacity
4. Calls `executeSling()` for each dispatch

### 4. Convoy Launch

**File:** `internal/cmd/convoy_launch.go:55-156`

Epic-driven batch dispatch:

```bash
gt convoy launch hq-cv-xyz
```

Dispatches Wave 1 beads, with subsequent waves auto-fed by daemon.

### 5. Witness Wake (Indirect)

**File:** `internal/cmd/sling_helpers.go:566-593`
**Function:** `wakeRigAgents()`

After polecat dispatch, wakes the rig's Witness:

```go
exec.Command("gt", "rig", "boot", rigName)  // Starts witness if needed
tmux.NudgeSession(witness, "Polecat dispatched - check for work")
```

---

## Session Creation Flow

### Master Function: `SessionManager.Start()`

**File:** `internal/polecat/session_manager.go:186-449`

This is the core orchestrator for polecat session creation.

#### Step-by-Step Breakdown:

| Lines | Action | Details |
|-------|--------|---------|
| 191-208 | Check existing session | Kill if stale |
| 210-241 | Load runtime config | Resolve agent settings |
| 244-249 | Ensure settings dir | Create runtime config directory |
| 251-266 | Create beacon | Work assignment prompt |
| 268-284 | Build startup command | Full command with env vars |
| 285-292 | Inject Dolt settings | `BD_DOLT_AUTO_COMMIT=off` |
| 307-317 | Inject GT vars | `GT_RIG`, `GT_POLECAT`, `GT_ROLE`, etc. |
| **321** | **CREATE TMUX SESSION** | `tmux.NewSessionWithCommand()` |
| 325-361 | Set session env vars | Non-fatal tmux set-environment |
| 368-374 | Hook issue | Attach work bead to polecat |
| 376-382 | Apply theming | Crash detection hook |
| 384-421 | Wait for ready | Claude startup + beacon nudges |
| 423-449 | Verify survival | Ensure session didn't crash |

### Tmux Session Creation

**File:** `internal/tmux/tmux.go:124-141`
**Function:** `NewSessionWithCommand()`

```bash
tmux -u new-session -d -s <sessionID> -c <workDir> <command>
```

| Flag | Purpose |
|------|---------|
| `-u` | UTF-8 mode |
| `-d` | Detached (no attach) |
| `-s` | Session name |
| `-c` | Working directory (polecat's worktree) |

### Session Naming Convention

**Format:** `gt-<rig-name>-<polecat-name>`

**Examples:**
- `gt-gastown-Toast`
- `gt-bcc-p-001`
- `gt-gastown-witness` (for Witness agent)

**Implementation:** `session.PolecatSessionName()` in `internal/session/`

---

## Command & Environment Injection

### Three-Stage Command Building

#### Stage 1: Base Command Structure

**File:** `internal/config/loader.go:2098`
**Function:** `BuildStartupCommandFromConfig()`

Builds the core command with OTEL context:
```
exec env GT_ROLE=... claude-code '<beacon-prompt>'
```

#### Stage 2: Environment Variables

**File:** `internal/config/env.go:63-254`
**Function:** `AgentEnv()`

Generates all environment variables for a polecat role:

| Variable | Value | Purpose |
|----------|-------|---------|
| `GT_ROLE` | `<rig>/polecats/<name>` | Full role identifier |
| `GT_RIG` | `<rig-name>` | Rig identifier |
| `GT_POLECAT` | `<polecat-name>` | Polecat identifier |
| `BD_ACTOR` | `<rig>/polecats/<name>` | Beads actor for commits |
| `GIT_AUTHOR_NAME` | `<polecat-name>` | Git commit attribution |
| `BD_DOLT_AUTO_COMMIT` | `off` | Disable auto-commit (batch mode) |

#### Stage 3: Additional Injection

**File:** `internal/polecat/session_manager.go:307-361`

Additional variables via `PrependEnv()`:

| Variable | Purpose |
|----------|---------|
| `GT_POLECAT_PATH` | Path to polecat's worktree |
| `GT_TOWN_ROOT` | Town workspace root |
| `GT_BRANCH` | Git branch name |
| `GT_AGENT` | Agent type (e.g., "claude") |
| `GT_PROCESS_NAMES` | Process names to monitor |

### Environment Prepending

**File:** `internal/config/loader.go:1956-1970`
**Function:** `PrependEnv()`

Transforms:
```
claude-code '<prompt>'
```

Into:
```bash
export GT_RIG=gastown GT_POLECAT=Toast GT_ROLE=gastown/polecats/Toast ... && claude-code '<prompt>'
```

### Beacon (Initial Prompt)

**File:** `internal/session/startup.go`
**Function:** `FormatStartupBeacon()`

The beacon is the initial prompt injected as CLI argument to Claude Code:

```
To: polecat/Toast/gastown
From: witness
Topic: assigned
Bead: gt-abc

Run `gt prime --hook` and begin work on gt-abc...
```

**Beacon Config Fields:**
- `Recipient`: Mail address (e.g., `polecat/Toast/gastown`)
- `Sender`: "witness"
- `Topic`: "assigned"
- `MolID`: The bead ID being worked
- `IncludePrimeInstruction`: Whether to include `gt prime --hook`
- `ExcludeWorkInstructions`: Whether to defer details via nudge

### Post-Creation Environment (tmux SetEnvironment)

**File:** `internal/polecat/session_manager.go:325-367`

After session creation, additional vars set via tmux (non-fatal):

```bash
tmux set-environment -t <session> GT_AGENT claude
tmux set-environment -t <session> GT_BRANCH feature-branch
tmux set-environment -t <session> GT_POLECAT_PATH /path/to/worktree
```

These allow respawned processes within the session to inherit context.

---

## Constraints & Settings

### max_polecats

#### Definition

**File:** `internal/scheduler/capacity/config.go:21`

```go
type SchedulerConfig struct {
    MaxPolecats *int `json:"max_polecats,omitempty"`
    BatchSize   *int `json:"batch_size,omitempty"`
    SpawnDelay  string `json:"spawn_delay,omitempty"`
}
```

#### Default Values

| Setting | Default | Meaning |
|---------|---------|---------|
| `max_polecats` | -1 | Direct dispatch (no capacity control) |
| `batch_size` | 1 | Dispatch 1 bead per heartbeat |
| `spawn_delay` | "0s" | No delay between spawns |

#### Semantics

| Value | Behavior |
|-------|----------|
| `-1` or `0` | Direct dispatch (backward compatible, no overhead) |
| `N > 0` | Deferred dispatch with capacity control (max N concurrent) |

#### Enforcement

**File:** `internal/cmd/capacity_dispatch.go:101-108`

```go
AvailableCapacity: func() (int, error) {
    active := countActivePolecats()  // via tmux list-sessions
    cap := maxPolecats - active
    if cap <= 0 {
        return 0, nil  // No free slots
    }
    return cap, nil
},
```

#### Active Polecat Counting

**File:** `internal/cmd/scheduler.go:484-506`

```go
func countActivePolecats() int {
    listCmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
    // Parse session names, count those with RolePolecat
}
```

### Settings Hierarchy

**Priority Order (first match wins):**

1. **Town-level settings** (highest): `<townRoot>/settings/config.json`
2. **Wisp layer** (ephemeral): `<townRoot>/.beads-wisp/config/<rigName>/settings.json`
3. **Rig identity bead labels**: `gt-<rig>` bead with `max_polecats:N`
4. **System defaults** (lowest): Compiled-in values

#### Town Settings File

**Path:** `<townRoot>/settings/config.json`

```json
{
  "type": "town-settings",
  "version": 1,
  "scheduler": {
    "max_polecats": 5,
    "batch_size": 2,
    "spawn_delay": "100ms"
  }
}
```

#### Rig-Level Defaults

**File:** `internal/rig/config.go:31-40`

```go
var SystemDefaults = map[string]interface{}{
    "status":       "operational",
    "auto_restart": true,
    "max_polecats": 10,  // RIG-LEVEL default
    "dnd":          false,
}
```

---

## Admission Control

### Pre-Spawn Checks

Before a polecat can be spawned, multiple admission checks occur:

#### 1. Parked Rig Check

**Locations:**
- `internal/cmd/sling_dispatch.go:103-107`
- `internal/cmd/sling_target.go:180-188`
- `internal/cmd/convoy_launch.go:105-156`

```go
if IsRigParked(townRoot, rigName) {
    return fmt.Errorf("cannot sling to parked rig %q", rigName)
}
```

**Implementation:** `internal/cmd/rig_park.go:179-184`

```go
func IsRigParked(townRoot, rigName string) bool {
    wispCfg := wisp.NewConfig(townRoot, rigName)
    return wispCfg.GetString("status") == "parked"
}
```

#### 2. Capacity Check

**File:** `internal/scheduler/capacity/pipeline.go:84-122`

```go
func PlanDispatch(availableCapacity, batchSize int, ready []PendingBead) DispatchPlan {
    if availableCapacity <= 0 {
        return DispatchPlan{Skipped: len(ready), Reason: "capacity"}
    }
    // Dispatch min(availableCapacity, batchSize, len(ready))
}
```

#### 3. Work Bead Readiness

**File:** `internal/cmd/capacity_dispatch.go:381-384`

```go
// Only include if work bead is ready (unblocked)
if !readyWorkIDs[fields.WorkBeadID] {
    continue
}
```

Ready beads are determined by `bd ready --json` which excludes beads with open blockers.

#### 4. Circuit Breaker

**File:** `internal/cmd/capacity_dispatch.go:376-379`

```go
const maxDispatchFailures = 3

if fields.DispatchFailures >= maxDispatchFailures {
    continue  // Skip beads that have failed 3+ times
}
```

#### 5. Database Connection Check

Before dispatch, queries must succeed:
- `bd ready --json --limit=0` (list ready work)
- `bd show <id>` (fetch bead details)

If Dolt is unavailable, dispatch is blocked.

---

## Scheduling Logic

### Direct vs Deferred Dispatch

**File:** `internal/cmd/sling_schedule.go:19-44`

```go
func shouldDeferDispatch() (bool, error) {
    settings := config.LoadOrCreateTownSettings(settingsPath)
    maxPol := settings.Scheduler.GetMaxPolecats()
    if maxPol > 0 {
        return true, nil   // Deferred: create sling context bead
    }
    return false, nil      // Direct: spawn immediately
}
```

### Deferred Dispatch Flow

When `max_polecats > 0`:

1. **Sling Context Bead Created** (`sling_schedule.go:64-197`)
   ```go
   fields := &capacity.SlingContextFields{
       WorkBeadID: beadID,
       TargetRig:  rigName,
       EnqueuedAt: time.Now().UTC().Format(time.RFC3339),
   }
   townBeads.CreateSlingContext(title, beadID, fields)
   ```

2. **Scheduler Heartbeat** (every 3 minutes)
   - Queries open sling contexts from HQ
   - Filters by readiness and capacity
   - Executes dispatch for available slots

3. **Capacity Planning** (`pipeline.go:88-122`)
   ```go
   toDispatch := min(availableCapacity, batchSize, len(ready))
   return DispatchPlan{ToDispatch: ready[:toDispatch]}
   ```

### Convoy Wave Dispatch

**File:** `internal/cmd/convoy_launch.go:55-156`

1. **Verify status**: staged_ready, staged_warnings, open, closed
2. **Check parked rigs** (block unless `--force`)
3. **Update status to "open"**
4. **Dispatch Wave 1** via capacity scheduler
5. **Subsequent waves** auto-fed by daemon based on DAG dependencies

### Scheduler State

**Path:** `<townRoot>/.runtime/scheduler-state.json`

```go
type SchedulerState struct {
    Paused            bool   `json:"paused"`
    PausedBy          string `json:"paused_by,omitempty"`
    PausedAt          string `json:"paused_at,omitempty"`
    LastDispatchAt    string `json:"last_dispatch_at,omitempty"`
    LastDispatchCount int    `json:"last_dispatch_count,omitempty"`
}
```

---

## Edge Cases & Failure Modes

### Zombie Polecat Classes

**File:** `internal/witness/handlers.go:980-1292`

| Class | Condition | Detection | Recovery |
|-------|-----------|-----------|----------|
| **Stuck-in-Done** | `gt done` hangs before cleanup | `done-intent` > 60s with live session | Kill + nuke |
| **Agent-Dead-in-Session** | Claude crashed, tmux alive | `IsAgentAlive()` returns false | Nuke + reset bead |
| **Bead-Closed-Still-Running** | Hooked bead closed but session alive | `getBeadStatus() == "closed"` | Nuke |
| **Agent-Hung** | No output for 30+ minutes | Activity timestamp check | Kill + reset bead |
| **Done-Intent-Dead** | Session dead but done-intent exists | No tmux + label present | Auto-nuke |

### Race Conditions

#### 1. TOCTOU Guard Pattern

**File:** `internal/witness/handlers.go:1005-1008, 1222-1235`

```go
detectedAt := time.Now()
// ... zombie detection ...
if sessionRecreated(t, sessionName, detectedAt) {
    return zombie, false  // Skip nuke if session recreated
}
```

**Risk:** Session creation time comparison may be unreliable with clock drift.

#### 2. Multiple Dispatch to Same Task

**File:** `internal/cmd/sling_dispatch.go:120-127`

When hooked agent is dead, auto-force is enabled:
```go
if info.Status == "hooked" && isHookedAgentDeadFn(info.Assignee) {
    params.Force = true  // Steal hook
}
```

**Risk:** Window between check and force where polecat could respawn.

#### 3. Hook Attachment Race

Between polecat spawn and hook attachment:
- If hook attachment fails and retries, old polecat may start without hook
- GUPP may not fire if hook not yet attached

### Session Creation Failures

**File:** `internal/session/lifecycle.go:136-264`

| Failure | Handling | Risk |
|---------|----------|------|
| tmux creation fails | Return error | Polecat allocated but never starts |
| RemainOnExit fails | Ignored (`_ =`) | Session may not persist for debugging |
| SetEnvironment fails | Ignored | Variables missing in session |
| WaitForAgent fails | May be ignored | Agent not ready but proceed |
| Theme application fails | Ignored | No crash detection hook |

**Impact:** Partial state leaves resources allocated but unusable.

### Cleanup Failures

**File:** `internal/witness/handlers.go:289-368`

| State | Behavior | Risk |
|-------|----------|------|
| `has_uncommitted` | Block nuke, escalate | Work may be lost |
| `has_stash` | Block nuke, escalate | State unclear |
| `has_unpushed` | Block nuke, "DO NOT NUKE" | Must push first |
| `unknown/empty` | Assume clean, auto-nuke | May lose work if parsing failed |

### Dolt Retry Logic Issues

**File:** `internal/polecat/manager.go:60-92`

- **Max retries:** 10
- **Base backoff:** 500ms
- **Max backoff:** 30s
- **Jitter:** ±25%

**Problem:** Config errors ("not initialized") detected and aborted, but Dolt may
still be starting up. Premature abort prevents waiting for bootstrap.

### Mail Routing Failures

**File:** `internal/witness/handlers.go:142-161`

If `nudgeRefinery()` fails:
- MERGE_READY not received by Refinery
- Work stalls until next patrol cycle
- No alarm raised

---

## Key Files Reference

### Core Session Management

| File | Purpose |
|------|---------|
| `internal/polecat/session_manager.go` | Master polecat session orchestration |
| `internal/tmux/tmux.go` | Raw tmux operations |
| `internal/session/lifecycle.go` | Session lifecycle primitives |
| `internal/session/startup.go` | Beacon formatting |

### Dispatch & Scheduling

| File | Purpose |
|------|---------|
| `internal/cmd/sling.go` | `gt sling` command |
| `internal/cmd/sling_dispatch.go` | Dispatch execution |
| `internal/cmd/sling_target.go` | Target resolution |
| `internal/cmd/sling_schedule.go` | Deferred dispatch scheduling |
| `internal/cmd/capacity_dispatch.go` | Daemon capacity scheduler |
| `internal/scheduler/capacity/pipeline.go` | Dispatch planning |
| `internal/scheduler/capacity/config.go` | Scheduler configuration |

### Configuration

| File | Purpose |
|------|---------|
| `internal/config/loader.go` | Config loading, command building |
| `internal/config/env.go` | Environment variable generation |
| `internal/config/types.go` | Config type definitions |
| `internal/rig/config.go` | Rig-level config |

### Monitoring & Cleanup

| File | Purpose |
|------|---------|
| `internal/witness/handlers.go` | Zombie detection, cleanup |
| `internal/witness/manager.go` | Witness lifecycle |
| `internal/cmd/rig_park.go` | Parked rig checks |

### Command Builders

| Function | File | Purpose |
|----------|------|---------|
| `BuildStartupCommandFromConfig` | config/loader.go:2098 | Full command with OTEL |
| `BuildStartupCommandWithAgentOverride` | config/loader.go:1979 | Command with agent resolution |
| `PrependEnv` | config/loader.go:1958 | Prepend `export ... &&` |
| `AgentEnv` | config/env.go:65 | Generate all env vars |
| `SessionManager.Start` | polecat/session_manager.go:186 | Main orchestrator |

---

## Appendix: Full Dispatch Chain

```
gt sling <bead> <rig>
    │
    ├─► runSling() [sling.go:161]
    │       │
    │       ├─► resolveTarget() [sling_target.go]
    │       │       │
    │       │       └─► IsRigName() → IsRigParked() check
    │       │
    │       └─► executeSling() [sling_dispatch.go]
    │               │
    │               ├─► SpawnPolecatForSling() [polecat_spawn.go]
    │               │       │
    │               │       ├─► AllocateName() or FindIdlePolecat()
    │               │       │
    │               │       ├─► AddWithOptions() or RepairWorktreeWithOptions()
    │               │       │
    │               │       └─► StartSession() [session_manager.go:186]
    │               │               │
    │               │               ├─► BuildStartupCommandFromConfig()
    │               │               │
    │               │               ├─► PrependEnv() [inject GT_*, BD_*]
    │               │               │
    │               │               ├─► tmux.NewSessionWithCommand() ◄── SESSION CREATED
    │               │               │
    │               │               ├─► tmux.SetEnvironment() [per var]
    │               │               │
    │               │               ├─► HookIssueToPolecat()
    │               │               │
    │               │               ├─► ApplyTheme() + crash detection
    │               │               │
    │               │               └─► WaitForReady() + NudgeSession()
    │               │
    │               └─► wakeRigAgents() [sling_helpers.go:566]
    │                       │
    │                       ├─► gt rig boot <rig>
    │                       │
    │                       └─► NudgeSession(witness, "Polecat dispatched...")
    │
    └─► [Polecat executes autonomously]
            │
            └─► gt done → MR → Refinery → MERGED → Nuke
```

---

*End of Polecat Lifecycle Technical Reference*
