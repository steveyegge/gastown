# Polecat Lifecycle Orchestration

> Formulas, hooks, daemons, and patrols that control polecat lifecycle

This document covers the **orchestration layer** - the formulas, hooks, daemons, and
patrols that control when polecats are spawned, monitored, and terminated.

---

## Overview: Orchestration Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DAEMON (3-min heartbeat)                        │
│  • Ensures Deacon, Witness, Refinery running                                 │
│  • Dispatches scheduled work (capacity-controlled)                           │
│  • Checks polecat session health                                             │
│  • Processes lifecycle requests (CYCLE, RESTART, SHUTDOWN)                   │
└─────────────────────────────────────────────────────────────────────────────┘
                    │                    │                    │
                    ▼                    ▼                    ▼
         ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
         │  DEACON PATROL   │  │  WITNESS PATROL  │  │ REFINERY PATROL  │
         │  (town-level)    │  │  (per-rig)       │  │  (per-rig)       │
         │  • Health checks │  │  • Polecat watch │  │  • MR processing │
         │  • Redispatch    │  │  • Zombie detect │  │  • MERGED signal │
         │  • Escalation    │  │  • Auto-nuke     │  │  • Conflict spawn│
         └──────────────────┘  └──────────────────┘  └──────────────────┘
                                        │
                                        ▼
         ┌──────────────────────────────────────────────────────────────┐
         │                    POLECAT FORMULAS                          │
         │  mol-polecat-work.formula.toml      (standard work)          │
         │  mol-polecat-lease.formula.toml     (witness tracking)       │
         │  mol-polecat-conflict.formula.toml  (conflict resolution)    │
         │  mol-polecat-review.formula.toml    (code review)            │
         └──────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
         ┌──────────────────────────────────────────────────────────────┐
         │                     CLAUDE CODE HOOKS                        │
         │  SessionStart: gt prime --hook (captures hooked work)        │
         │  PreCompact: gt prime --hook (maintains work across cycle)   │
         │  UserPromptSubmit: gt mail check --inject                    │
         │  Stop: gt costs record                                       │
         └──────────────────────────────────────────────────────────────┘
```

---

## 1. Daemon: Heartbeat and Dispatch

**File:** `internal/daemon/daemon.go`

### Heartbeat Configuration

| Setting | Value | Line |
|---------|-------|------|
| Recovery heartbeat | 3 minutes | 251 |
| Hung session threshold | 30 minutes | 57-70 |
| Mass death window | 30 seconds | 57 |
| Mass death threshold | 3 deaths | 58 |

### Heartbeat Actions (Lines 407-515)

Each 3-minute heartbeat performs:

| Order | Action | Polecat Impact |
|-------|--------|----------------|
| 1 | Ensure Dolt server | Dependency for beads |
| 2 | Ensure Deacon | Town-level watchdog |
| 3 | Poke Boot for Deacon | Intelligent triage |
| 4 | Deacon heartbeat check | Fallback direct check |
| 5 | Ensure Witnesses | Per-rig polecat monitors |
| 6 | Ensure Refineries | MR processors |
| 7 | Ensure Mayor | Town coordinator |
| 8 | Handle Dogs | Cross-rig infrastructure |
| 9 | **Process lifecycle requests** | CYCLE/RESTART/SHUTDOWN |
| 10 | Check GUPP violations | Work-on-hook not progressing |
| 11 | Check orphaned work | Assigned to dead agents |
| 12 | **Check polecat session health** | Proactive crash detection |
| 13 | Clean orphaned processes | Process cleanup |
| 14 | Prune stale branches | Git cleanup |
| 15 | **DISPATCH SCHEDULED WORK** | Capacity-controlled spawn |

### Capacity Dispatch (Lines 1751-1780)

```go
func (d *Daemon) dispatchQueuedWork() {
    // Shell out to `gt scheduler run` with 5-minute timeout
    cmd := exec.Command("gt", "scheduler", "run")
    // Batch size limits in-flight beads per heartbeat
}
```

**Dispatch modes:**
- `max_polecats <= 0`: Direct dispatch (no queueing)
- `max_polecats > 0`: Capacity-controlled (queue excess)

---

## 2. Witness Patrol: Polecat Monitoring

**File:** `internal/formula/formulas/mol-witness-patrol.formula.toml`

### Patrol Steps

| Step | Purpose | Polecat Impact |
|------|---------|----------------|
| inbox-check | Process mail | POLECAT_DONE → auto-nuke if clean |
| process-cleanups | Handle dirty polecats | Escalate to Mayor |
| check-refinery | Health check | Ensure merge queue works |
| **survey-workers** | Inspect polecats | Zombie detection |
| check-timer-gates | Async gates | Watch for stuck gates |
| check-swarm-completion | Batch monitor | Wave completion |
| patrol-cleanup | Inbox hygiene | Clean up |
| loop-or-exit | Continue/respawn | Patrol continuity |

### Zombie Detection (survey-workers step)

**Three-Level Health Check:**

```
Level 1: Does tmux session exist?
    └─► NO → SessionDead (ZOMBIE)

Level 2: Is agent process alive inside session?
    └─► NO → AgentDead (session exists, process dead)

Level 3: Has activity occurred in last 30 min?
    └─► NO → AgentHung (alive but stuck)
```

**Implementation:** `internal/tmux/tmux.go:1286-1310`

```go
func (t *Tmux) CheckSessionHealth(sessionName string, hungThreshold time.Duration) ZombieStatus {
    // Level 1: Session exists?
    if !t.HasSession(sessionName) {
        return SessionDead
    }
    // Level 2: Agent process alive?
    if !t.IsAgentAlive(sessionName) {
        return AgentDead
    }
    // Level 3: Recent activity?
    if t.GetLastActivityAge(sessionName) > hungThreshold {
        return AgentHung
    }
    return SessionHealthy
}
```

### Auto-Nuke Triggers

| Condition | Action |
|-----------|--------|
| POLECAT_DONE + cleanup_status=clean | Immediate auto-nuke |
| Idle polecat + clean git state | Auto-nuke in survey |
| Zombie + clean git state | Auto-nuke |
| Zombie + dirty git state | Escalate to Deacon/Mayor |

### POLECAT_DONE Handler

**File:** `internal/witness/handlers.go:HandlePolecatDone`

```go
func HandlePolecatDone(msg *mail.Message) {
    status := parseCleanupStatus(msg)

    if status == "clean" {
        // Branch pushed, ready to nuke
        gt polecat nuke <name>
        archiveMail(msg)
    } else {
        // Dirty state - create cleanup wisp
        createCleanupWisp(polecatName, status)
    }
}
```

---

## 3. Deacon Patrol: System-Wide Monitoring

**File:** `internal/formula/formulas/mol-deacon-patrol.formula.toml`

### Key Responsibilities

| Responsibility | Action |
|----------------|--------|
| Health checks | Send HEALTH_CHECK nudges to agents |
| Force-kill | After 3 consecutive failures |
| Stale hook cleanup | Unhook beads > 1 hour with dead agent |
| Redispatch | Re-dispatch recovered beads |

### Health Check Failure Escalation

```
1-2 failures: Note it, continue
3-4 failures: Attempt restart
5+ failures: Escalate to Mayor
```

### Redispatch (RECOVERED_BEAD)

When Witness recovers a bead from a dead polecat:

```bash
gt mail send deacon/ -s "RECOVERED_BEAD <bead-id>" -m "..."
# Deacon receives, auto re-dispatches:
gt deacon redispatch <bead-id>
```

---

## 4. Refinery Patrol: MR Processing

**File:** `internal/formula/formulas/mol-refinery-patrol.formula.toml`

### Polecat Lifecycle Touchpoints

| Step | Polecat Impact |
|------|----------------|
| MERGE_READY receipt | Signals polecat work complete |
| Conflict detection | **Spawns NEW polecat** for resolution |
| MERGED signal | Triggers witness to nuke original polecat |

### Conflict Resolution: Fresh Polecat Spawn

When mechanical rebase fails:

```bash
# Refinery creates conflict-resolution task
bd create --type=task "Resolve conflicts for <mr-id>"

# Task includes metadata:
# - original_mr: <mr-bead-id>
# - conflict_sha: <sha>
# - branch: <polecat-branch>

# Daemon picks up, spawns FRESH polecat
gt sling <task-id> <rig>
```

**Critical:** Original polecat is already gone. Conflict polecat is NEW instance.

### MERGED Signal (Critical)

```bash
# Sent IMMEDIATELY after successful push (before any cleanup)
gt mail send <rig>/witness -s "MERGED <polecat-name>" -m "
Branch: <branch>
Issue: <issue-id>
Merged-At: $(date -u +%Y-%m-%dT%H:%M:%SZ)
"
```

**Why immediate?** If Refinery crashes after push but before signal,
polecat worktrees would accumulate indefinitely.

---

## 5. Polecat Work Formulas

### mol-polecat-work.formula.toml

**Location:** `.beads/formulas/mol-polecat-work.formula.toml`

**Self-Cleaning Model:**

```
Polecat lifecycle: spawned → working → gt done → GONE
                                         │
                                         ├─► Push branch
                                         ├─► Create MR bead
                                         ├─► Nuke sandbox
                                         └─► Exit session
```

**Key Principle:** "Done means gone" - no idle state, no waiting for merge.

**Completion Flow:**
1. Push branch to origin
2. Create MR bead in merge queue
3. Run `gt done` which nukes sandbox and exits

**NOT Polecat Responsibilities:**
- Do NOT push to main (Refinery merges)
- Do NOT close own issue (Refinery closes)
- Do NOT wait for merge (gone after `gt done`)
- Do NOT handle rebase conflicts (fresh polecat spawned)

### mol-polecat-lease.formula.toml

**Witness-side tracking of polecat lifecycle:**

```
BOOT → WORKING → VERIFYING → MERGE_REQUESTED → DONE
  │       │           │              │
  └─ STUCK ┴─ STUCK ──┴──► STUCK ───┘
```

| State | Witness Actions |
|-------|-----------------|
| BOOT | Verify polecat starts, nudge if idle > 60s |
| WORKING | Monitor progress, watch for stuck/completion |
| VERIFYING | Verify merge-ready (clean git state) |
| MERGE_REQUESTED | Send MERGE_READY, await MERGED |
| DONE | Kill session, remove worktree, close lease |

### mol-polecat-conflict.formula.toml

**Conflict resolution polecat (direct push):**

```bash
# Acquire merge slot (exclusive)
bd merge-slot acquire --holder=$(whoami) --wait --json

# Resolve conflicts, push directly to main
git push origin main

# Release slot
bd merge-slot release --holder=$(whoami) --json

# Close original MR
bd close {{original_mr}} --reason="merged after conflict resolution"
```

**Key Difference:** Unlike normal polecats, conflict polecats push directly to main.

---

## 6. Claude Code Hooks

**File:** `internal/hooks/config.go`

### Default Base Hooks

| Event | Command | Polecat Impact |
|-------|---------|----------------|
| **SessionStart** | `gt prime --hook` | **CRITICAL** - captures hooked work |
| **PreCompact** | `gt prime --hook` | Maintains work across compaction |
| UserPromptSubmit | `gt mail check --inject` | Enables mail coordination |
| Stop | `gt costs record` | Records API usage |
| PreToolUse | `gt tap guard pr-workflow` | Prevents invalid branches |

### SessionStart Hook (Lines 575-584)

```json
{
  "event": "SessionStart",
  "hooks": [{
    "matcher": "",
    "command": "export PATH=... && gt prime --hook"
  }]
}
```

**Why Critical:** Without this hook, polecat won't discover its hooked work.
The `--hook` flag ensures session_start events are emitted for seance discovery.

### PreCompact Hook (Lines 586-595)

```json
{
  "event": "PreCompact",
  "hooks": [{
    "matcher": "",
    "command": "export PATH=... && gt prime --hook"
  }]
}
```

**Why Critical:** When context compacts, polecat needs to rediscover its work.
Hook data persists in Dolt (agent_bead.hook_bead), not conversation context.

### Hook Installation

**Target for polecats:** `<rig>/polecats/.claude/settings.json` (shared)

All polecats in a rig share ONE settings.json in the polecats parent directory.

---

## 7. Git Hooks

**File:** `.githooks/pre-push`

### Branch Enforcement (Lines 1-99)

```bash
# Allowed branches
ALLOWED_BRANCHES="main|polecat/.*|integration/.*|beads-sync"

# Block arbitrary feature branches
if [[ ! "$branch" =~ ^($ALLOWED_BRANCHES)$ ]]; then
    echo "ERROR: Branch '$branch' not allowed"
    exit 1
fi
```

**Polecat Impact:** Polecats MUST push to `polecat/<name>/*` branches.

### Integration Branch Guard (Lines 62-95)

```bash
# Block accidental integration branch landing
if [[ "$TARGET_BRANCH" == "$DEFAULT_BRANCH" ]]; then
    # Check for integration branch content
    if git log ... | grep -q "integration/"; then
        echo "ERROR: Set GT_INTEGRATION_LAND=1 to land integration branch"
        exit 1
    fi
fi
```

---

## 8. Rig Setup Hooks

**File:** `internal/rig/setuphooks.go:40-97`

### Hook Location

```
<rigPath>/.runtime/setup-hooks/
├── 00-install-deps.sh
├── 10-configure-db.sh
└── 20-seed-data.sh
```

### Execution

- **Order:** Alphabetical
- **Working directory:** Polecat worktree
- **Timeout:** 60 seconds per hook
- **Failure:** Logged as warning, doesn't fail spawn

### Environment Variables

| Variable | Value |
|----------|-------|
| `GT_WORKTREE_PATH` | Path to polecat worktree |
| `GT_RIG_PATH` | Path to rig root |

---

## 9. Lifecycle State Machine

### Polecat States

```go
// internal/polecat/types.go:26-54
const (
    StateWorking = "working"  // Active, hook_bead set
    StateIdle    = "idle"     // Complete, hook_bead cleared, sandbox preserved
    StateDone    = "done"     // Calling gt done (transient)
    StateStuck   = "stuck"    // Needs help
    StateZombie  = "zombie"   // Tmux exists, no worktree
)
```

### State Transitions

```
          ┌─────────────────────────────────────────┐
          │                                         │
          ▼                                         │
     [SPAWNED] ──► [WORKING] ──► [DONE] ──► [NUKED]│
          │            │            │               │
          │            ▼            │               │
          │        [STUCK] ────────►┤               │
          │            │            │               │
          │            ▼            │               │
          │       [ESCALATED] ─────►┤               │
          │                         │               │
          └─────────────────────────┘               │
                                                    │
     [IDLE] (reusable) ◄────────────────────────────┘
        │
        └──► [WORKING] (via RepairWorktreeWithOptions)
```

### Hook Bead Atomicity

**File:** `internal/polecat/manager.go:769-777, 1239-1246`

```go
// Set atomically at creation
m.createAgentBeadWithRetry(agentID, &beads.AgentFields{
    RoleType:   "polecat",
    Rig:        m.rig.Name,
    AgentState: "spawning",
    HookBead:   opts.HookBead,  // ← ATOMIC
})
```

**Why Atomic:** Prevents race where polecat starts before work is assigned.

---

## 10. Summary: What Controls Polecat Lifecycle

### Spawning Triggers

| Trigger | Component | Mechanism |
|---------|-----------|-----------|
| `gt sling <bead> <rig>` | User command | Direct or queued |
| Scheduler heartbeat | Daemon | Capacity-controlled dispatch |
| Convoy launch | Convoy manager | Wave 1 dispatch |
| Conflict resolution | Refinery | Fresh polecat for conflicts |

### Monitoring Systems

| System | Scope | Checks |
|--------|-------|--------|
| Daemon | Town | Session health, GUPP violations |
| Deacon | Town | Health checks, stale hooks |
| Witness | Per-rig | Zombie detection, cleanup |

### Termination Triggers

| Trigger | Handler | Action |
|---------|---------|--------|
| `gt done` (clean) | Witness | Auto-nuke |
| Zombie (clean) | Witness | Auto-nuke |
| Zombie (dirty) | Witness → Deacon | Escalate |
| MERGED signal | Witness | Auto-nuke |
| Health check failure (3x) | Deacon | Force-kill |
| Stuck > 30 min | Daemon | Kill session |

### Critical Signals

| Signal | Sender | Receiver | Purpose |
|--------|--------|----------|---------|
| POLECAT_DONE | Polecat | Witness | Completion notification |
| MERGE_READY | Witness | Refinery | MR ready for processing |
| MERGED | Refinery | Witness | Trigger nuke |
| RECOVERED_BEAD | Witness | Deacon | Redispatch recovered work |
| HEALTH_CHECK | Deacon | Agent | Liveness probe |
| LIFECYCLE:* | Various | Various | Cycle/Restart/Shutdown |
