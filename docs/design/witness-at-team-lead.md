# Witness AT Team Lead: Implementation Spec

> **Bead:** gt-ky4jf
> **Date:** 2026-02-08
> **Author:** furiosa (gastown polecat)
> **Depends on:** AT spike report (gt-3nqoz), AT integration design (agent-teams-integration.md)
> **Status:** Phase 1 implementation spec

---

## Overview

This document specifies how the Witness becomes an AT team lead, replacing the
current tmux-based polecat session management with Claude Code Agent Teams.

The Witness enters delegate mode (structurally enforced coordination-only), spawns
polecat teammates for assigned work, monitors them via AT's native lifecycle hooks,
and syncs completions to beads at task boundaries.

**What changes:** Session management layer (tmux → AT).
**What stays:** Beads as ledger, gt mail for cross-rig, molecules/formulas, `gt done`.

---

## 1. Witness in Delegate Mode

### What Tools the Witness Keeps

In delegate mode, the Witness has access to:

| Tool | Purpose |
|------|---------|
| `Teammate` | Spawn/shutdown teammates, send messages, manage team |
| `TaskCreate` | Create AT tasks for polecat work |
| `TaskUpdate` | Update task status, set dependencies |
| `TaskList` | Monitor team progress |
| `TaskGet` | Read task details |
| `Bash` | **Not available** in delegate mode |
| `Read/Write/Edit` | **Not available** in delegate mode |
| `Glob/Grep` | **Not available** in delegate mode |

### The ZFC Upgrade

Current state: "Witness doesn't implement" is enforced by CLAUDE.md instructions.
Agents can and do violate this under pressure.

New state: Delegate mode structurally removes implementation tools. The Witness
literally *cannot* edit files. This is the strongest possible ZFC compliance —
the constraint is in the machinery, not in the instructions.

### Witness Needs Bash for gt/bd Commands

**Problem:** Delegate mode removes Bash access, but the Witness needs to run
`gt mail`, `bd show`, `bd close`, and other coordination commands.

**Solution options (in order of preference):**

1. **Custom agent definition with selective tools.** Create
   `.claude/agents/witness-lead.md` that uses `permissionMode: delegate` but
   adds back Bash via the `tools` allowlist. This gives structural enforcement
   for file editing while preserving command access:

   ```yaml
   ---
   name: witness-lead
   permissionMode: delegate
   tools: Teammate, TaskCreate, TaskUpdate, TaskList, TaskGet, Bash
   ---
   ```

   **Risk:** Bash access means the Witness *could* edit files via sed/echo.
   Mitigated by: PreToolUse hook on Bash that rejects file-modifying commands.

2. **Hooks as command proxy.** The Witness doesn't run commands directly.
   Instead, hooks fire at turn boundaries and execute gt/bd commands based on
   AT task state. The Witness coordinates purely through AT tools; the hooks
   handle the beads bridge.

   **Risk:** Less flexible — Witness can't make ad-hoc bd queries. But it's
   the purest delegate mode implementation.

3. **Teammate as command runner.** Spawn a lightweight "ops" teammate whose
   sole job is running gt/bd commands on the Witness's behalf. The Witness
   sends commands via AT messaging; the ops teammate executes and returns results.

   **Risk:** Token overhead for a simple command proxy. But it preserves
   strict delegate mode for the Witness.

**Recommendation:** Option 1 (custom agent with selective tools). It's pragmatic,
preserves the Witness's ability to query beads state, and the PreToolUse hook
provides sufficient guardrails. Pure delegate mode is aspirational but the
Witness genuinely needs to read beads state for coordination decisions.

### PreToolUse Guard for Witness Bash

```json
{
  "PreToolUse": [{
    "matcher": "Bash",
    "hooks": [{
      "type": "command",
      "command": "gt witness-bash-guard"
    }]
  }]
}
```

The `gt witness-bash-guard` script:
- Allows: `gt *`, `bd *`, `git status`, `git log`, read-only commands
- Blocks: `echo >`, `cat >`, `sed -i`, `vim`, `nano`, any write operation
- Returns exit code 2 with reason on block

---

## 2. Teammate Spawn: Work Assignment → AT Task Creation

### The Spawn Flow

When work arrives (via convoy, gt sling, or direct assignment):

```
1. Witness receives work (mail, convoy dispatch, bd ready)
2. Witness creates AT team (if not already active)
3. For each issue to dispatch:
   a. Create AT task with issue details and dependencies
   b. Spawn polecat teammate assigned to that task
4. Teammates self-claim tasks and begin execution
```

### Team Creation

```
Teammate({
  operation: "spawnTeam",
  team_name: "<rig-name>-work",
  description: "Polecat work team for <convoy/sprint description>"
})
```

Team naming convention: `<rig>-work` for the primary work team.
One team per rig per active convoy. Multiple convoys = multiple teams
(AT limitation: one team per session, so Witness manages one convoy
at a time).

### AT Task Creation from Beads Issues

For each issue dispatched to a polecat:

```
TaskCreate({
  subject: "<issue title>",
  description: "Issue: <issue-id>\n<issue description>\n\nWorktree: /path/to/<polecat>/\nFormula: mol-polecat-work",
  activeForm: "Working on <issue title>",
  metadata: {
    "bead_id": "<issue-id>",
    "worktree": "/path/to/worktree",
    "molecule": "<mol-id>"
  }
})
```

**Key fields in metadata:**
- `bead_id`: Links AT task back to the beads issue for sync
- `worktree`: The git worktree path this polecat should use
- `molecule`: The mol-polecat-work instance for this issue

### Dependency Mapping

Beads issue dependencies map to AT task dependencies:

```
# If issue B depends on issue A:
# After creating both tasks:
TaskUpdate({
  taskId: "<task-B-id>",
  addBlockedBy: ["<task-A-id>"]
})
```

This enables AT's native self-claim: when task A completes, task B becomes
unblocked and the next idle teammate picks it up automatically.

### Polecat Teammate Spawn

```
Task({
  subagent_type: "polecat",
  team_name: "<rig>-work",
  name: "<polecat-name>",
  model: "sonnet",
  prompt: "You are polecat <name>. Your worktree is <path>.\n\nAssigned issue: <id> - <title>\n<description>\n\nWorkflow:\n1. cd <worktree>\n2. Run `gt prime` for full context\n3. Follow mol-polecat-work steps\n4. When done: commit, push, run `gt done`"
})
```

**Model selection:**
- Polecat teammates: `model: "sonnet"` (execution-focused, cost-efficient)
- Witness lead: Opus (judgment, coordination, quality review)
- Refinery teammate (Phase 2): `model: "sonnet"` (mechanical merge work)

### The `.claude/agents/polecat.md` Definition

```yaml
---
name: polecat
description: Gas Town polecat worker agent (persistent identity, ephemeral sessions)
model: sonnet
hooks:
  SessionStart:
    - hooks:
        - type: command
          command: "export PATH=\"$HOME/go/bin:$HOME/.local/bin:$PATH\" && gt prime --hook"
  PreToolUse:
    - matcher: "Write|Edit"
      hooks:
        - type: command
          command: "gt validate-worktree-scope"
  PreCompact:
    - matcher: "auto"
      hooks:
        - type: command
          command: "gt handoff --reason compaction"
  Stop:
    - hooks:
        - type: command
          command: "gt signal stop"
---

You are a Gas Town polecat (persistent identity, ephemeral sessions).

## Startup
1. `cd` to your assigned worktree (given in your spawn prompt)
2. Run `gt prime` for full context
3. Check your hook: `gt hook`
4. Follow molecule steps: `bd ready`

## Work Protocol
- Mark steps in_progress before starting: `bd update <id> --status=in_progress`
- Close steps when done: `bd close <id>`
- Commit frequently with descriptive messages
- Never batch-close steps

## Completion
When all steps done:
1. `git status` — must be clean
2. `git push`
3. `gt done` — submits to merge queue, nukes your sandbox
```

### Worktree Assignment

Each polecat teammate operates in its own git worktree. Since AT doesn't support
per-teammate working directories natively, enforcement is via:

1. **Spawn prompt:** First instruction is `cd /path/to/worktree`
2. **PreToolUse hook:** `gt validate-worktree-scope` rejects Write/Edit operations
   targeting paths outside the assigned worktree
3. **Environment variable:** `GT_WORKTREE=/path/to/worktree` set via SessionStart hook

The Witness creates worktrees before spawning teammates:
```bash
git worktree add /path/to/polecats/<name>/<rig> -b polecat/<name>/<issue-id>
```

This matches the current worktree management — the change is WHO creates them
(Witness via AT, not `gt sling` via Go daemon).

---

## 3. Bead Sync Protocol

### The Two-Layer Model

```
Layer 1 (AT, ephemeral):     Task claiming, status, messaging
Layer 2 (Beads/Dolt, durable): Issue creation, completion, audit trail
```

### Sync Points

| AT Event | Beads Action | Trigger |
|----------|-------------|---------|
| Task claimed (in_progress) | `bd update <id> --status=in_progress` | TaskCompleted hook / polecat prompt |
| Task completed | `bd close <step-id>` | TaskCompleted hook |
| New issue discovered | AT task created by Witness | Witness reads polecat message |
| Teammate idle | Check beads for more work | TeammateIdle hook |
| Team shutdown | Verify all beads synced | Witness cleanup routine |

### TaskCompleted Hook for Bead Sync

The `TaskCompleted` hook fires when an AT task is marked complete. This is the
primary sync mechanism:

```bash
#!/bin/bash
# .claude/hooks/task-completed-sync.sh
# Fires on TaskCompleted hook

BEAD_ID=$(echo "$TASK_METADATA" | jq -r '.bead_id // empty')
if [ -n "$BEAD_ID" ]; then
  export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"
  bd close "$BEAD_ID" 2>/dev/null
fi
exit 0
```

Hook configuration:
```json
{
  "TaskCompleted": [{
    "hooks": [{
      "type": "command",
      "command": ".claude/hooks/task-completed-sync.sh"
    }]
  }]
}
```

**Important:** The hook should NOT block task completion (exit 0 always). If the
`bd close` fails (Dolt contention), it will be retried at the next sync point.
The AT task list is the real-time truth; beads catches up at boundaries.

### Polecat-Side Bead Updates

Polecats still run `bd update` and `bd close` directly as part of their molecule
workflow. The TaskCompleted hook is a safety net, not the primary mechanism. This
means:

- Polecat marks molecule step in_progress → `bd update --status=in_progress`
- Polecat completes molecule step → `bd close <step-id>`
- AT task completion → TaskCompleted hook also fires `bd close` (idempotent)

Double-close is safe: `bd close` on an already-closed bead is a no-op.

### Sync Verification at Team Shutdown

Before the Witness shuts down the team, it verifies beads are in sync:

```
For each AT task marked completed:
  1. Read task metadata for bead_id
  2. Verify bead is closed (bd show <id> | check status)
  3. If bead still open: bd close <id> with notes
  4. If close fails: log warning, continue (Dolt retry will handle)
```

This is the "boundary sync" pattern from the integration design: AT handles
real-time coordination, beads catches up at lifecycle boundaries (team shutdown,
convoy completion).

---

## 4. Session Cycling: Compaction → Respawn → Resume

### The Problem

AT teammates cannot be resumed after shutdown. When a teammate hits context
limits and compacts, or crashes, a new teammate must be spawned.

### The Lifecycle

```
Teammate running
    │
    ├── Context filling → PreCompact hook fires
    │   │
    │   └── gt handoff --reason compaction
    │       ├── Saves current molecule step to beads
    │       ├── Saves progress notes
    │       └── Saves git branch state
    │
    ├── Auto-compaction occurs
    │   │
    │   └── SessionStart hook fires (source: "compact")
    │       └── gt prime --compact-resume
    │           └── Reads beads state, restores context
    │
    └── Teammate continues with compressed context
```

### When Compaction Isn't Enough (Teammate Death)

If a teammate crashes or is shut down (not just compacted):

```
Teammate stops
    │
    └── SubagentStop hook fires on Witness (lead)
        │
        ├── Read teammate's last known state from beads
        │   └── Which molecule step was in_progress?
        │   └── What branch was being worked on?
        │
        ├── Assess: recoverable or escalate?
        │   ├── Normal completion: AT task done, beads synced → no action
        │   ├── Incomplete work: respawn with resume context
        │   └── Repeated crashes: escalate to Witness mail → Mayor
        │
        └── If recoverable: spawn replacement teammate
            └── Task({ subagent_type: "polecat", ... resume prompt ... })
```

### SubagentStop Hook (Witness Side)

```json
{
  "SubagentStop": [{
    "matcher": "polecat",
    "hooks": [{
      "type": "command",
      "command": "gt witness-teammate-stopped"
    }]
  }]
}
```

The `gt witness-teammate-stopped` script:
1. Reads the stopped agent's transcript path (available in hook input)
2. Checks AT task status — was the task completed?
3. Checks beads — was `gt done` run?
4. If completed: no action (normal lifecycle)
5. If incomplete: outputs `{ "decision": "block", "reason": "Teammate <name> stopped before completing task <id>. Beads state: <status>. Respawn needed." }`

The "block" decision prevents the Witness from going idle, injecting the
respawn instruction as context for the Witness to act on.

### Respawn Prompt Template

```
Teammate <name> stopped before completing work.

Last known state:
- Issue: <bead-id> (<title>)
- Molecule step: <step-id> (in_progress)
- Branch: <branch-name>
- Worktree: <path>

Spawn a replacement polecat with this context. The new teammate
should read beads state and continue from the last checkpoint.
```

### Crash Loop Prevention

Track respawn attempts per issue. If a teammate crashes 3 times on the
same issue:

1. Mark the AT task as blocked
2. File a bead: `bd create --title "Polecat crash loop on <issue>" --type bug`
3. Mail the Witness/Mayor for escalation
4. Do NOT respawn — the issue has a structural problem

Tracking: Use AT task metadata `{ "respawn_count": N }` incremented on
each respawn. This is ephemeral (dies with the team) which is correct —
crash tracking only matters during the current team session.

---

## 5. Error Handling

### Error Categories and Responses

| Error | Detection | Response |
|-------|-----------|----------|
| Teammate crash | SubagentStop hook | Respawn or escalate (see above) |
| Teammate stuck (no progress) | TeammateIdle hook | Send message asking for status |
| Test failures | TaskCompleted hook (exit 2) | Block completion, teammate must fix |
| Merge conflict | Polecat messages Witness | Witness advises or reassigns |
| Dolt write failure | bd command exit code | Retry with backoff (existing mechanism) |
| AT team crash | Witness session dies | Daemon/Boot/Deacon chain detects, restarts Witness |
| Worktree scope violation | PreToolUse hook | Block the operation, warn polecat |

### TeammateIdle Hook

```bash
#!/bin/bash
# gt witness-teammate-idle
# Fires when a teammate is about to go idle

export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

# Check if there's more work in beads
READY=$(bd ready --count 2>/dev/null)
if [ "$READY" -gt 0 ]; then
  echo "There is more work available. Run 'bd ready' to see unblocked tasks." >&2
  exit 2  # Block idle, send feedback
fi

# Check if gt done was run
if git log --oneline -1 | grep -q "gt done"; then
  exit 0  # Normal completion
fi

# Teammate seems genuinely idle without completing
echo "Your work doesn't appear complete. Run 'bd ready' to check remaining steps, or 'gt done' if finished." >&2
exit 2
```

### TaskCompleted Quality Gate

```bash
#!/bin/bash
# Fires on TaskCompleted hook
# Validates that work meets minimum quality before marking complete

export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

# Check for uncommitted changes
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
  echo "Uncommitted changes detected. Commit your work before marking complete." >&2
  exit 2
fi

# Check that the branch has been pushed
BRANCH=$(git branch --show-current 2>/dev/null)
if ! git log "origin/$BRANCH" --oneline -1 >/dev/null 2>&1; then
  echo "Branch not pushed to remote. Run 'git push' before completing." >&2
  exit 2
fi

exit 0
```

---

## 6. Convoy Mapping to AT Teams

### The Natural Mapping

| Gas Town | AT Equivalent |
|----------|--------------|
| Convoy | AT team lifecycle |
| Convoy issues | AT tasks |
| War Rig (per-rig convoy execution) | AT team instance |
| Ready front (unblocked issues) | Unblocked AT tasks |
| Dispatch | AT task creation + teammate spawn |
| Completion tracking | AT task list status |

### One Convoy = One AT Team Session

A convoy arrives at a rig. The Witness creates an AT team for that convoy:

```
Convoy hq-abc arrives at gastown
    │
    ├── Witness creates team: "gastown-convoy-abc"
    │
    ├── For each issue in convoy:
    │   ├── Create AT task (with bead_id in metadata)
    │   └── Set dependencies (from beads dep graph)
    │
    ├── Spawn N polecat teammates (N = min(issues, max_polecats))
    │
    ├── Teammates self-claim tasks from ready front
    │
    ├── As tasks complete:
    │   ├── Dependencies unblock next tasks
    │   ├── Idle teammates auto-claim newly ready tasks
    │   └── Beads synced via TaskCompleted hook
    │
    └── All tasks done:
        ├── Witness verifies beads sync
        ├── Witness sends convoy completion to Mayor (gt mail)
        └── Team shutdown
```

### Multiple Convoys

AT limitation: one team per session. If a second convoy arrives while the
first is active:

**Option A: Sequential processing.** Finish convoy 1, then start convoy 2.
Simple, no concurrency issues. Acceptable if convoy throughput is sufficient.

**Option B: Convoy queue.** The Witness queues incoming convoys and processes
them in order. The queue lives in beads (mail inbox) — the Witness checks for
new convoys when the current team finishes.

**Option C: Multiple Witness sessions.** The daemon spawns a second Witness
session for the second convoy. Each Witness manages its own AT team. This
requires the daemon to support multiple Witness instances per rig.

**Recommendation:** Option A for Phase 1 (sequential). Option C for Phase 2+
if throughput demands it. The convoy queue in Option B is implicit in beads
already (unprocessed convoy mail = queued work).

### Steady-State Worker Pool

For large convoys (20+ issues), the Witness doesn't spawn 20 teammates at once.
Instead:

```
max_teammates = 5  # configurable per rig

1. Spawn max_teammates polecats
2. Create all AT tasks (with dependencies)
3. Teammates self-claim from ready front
4. As teammates complete tasks:
   - Auto-claim next unblocked task
   - No respawn needed (same teammate, new task)
5. When all tasks done: team shutdown
```

AT's self-claim mechanism is the key enabler. Teammates don't die after one
task — they pick up the next one. This eliminates the current spawn/nuke
overhead per issue.

**When a teammate needs to cycle** (compaction), the Witness spawns a
replacement, not an additional teammate. The pool size stays at max_teammates.

---

## 7. Mail Bridge: gt mail ↔ AT Messages

### The Boundary

```
                    ┌─────────────────┐
                    │    Witness       │
                    │  (AT Team Lead)  │
                    │                  │
    gt mail ←──────│── Bridge ──────→ AT messaging
    (cross-rig,    │                  (intra-team,
     persistent)   │                   ephemeral)
                    └─────────────────┘
```

### Inbound: gt mail → AT message

When the Witness receives gt mail relevant to an active teammate:

```
gt mail inbox
    │
    ├── From Mayor: "Priority shift — issue X is now P0"
    │   └── Witness sends AT message to relevant teammate:
    │       Teammate({ operation: "write", target_agent_id: "<polecat>",
    │                  value: "Priority update: <issue> is now P0. Expedite." })
    │
    ├── From Refinery: "Merge conflict on <branch>"
    │   └── Witness sends AT message to the polecat on that branch:
    │       Teammate({ operation: "write", target_agent_id: "<polecat>",
    │                  value: "Merge conflict detected. Rebase on main." })
    │
    └── From another rig's Witness: "Dependency <issue> is done"
        └── Witness creates/unblocks AT task for downstream work
```

### Outbound: AT event → gt mail

When AT events need to reach entities outside the team:

```
Teammate completes final task
    │
    └── Witness detects all tasks done
        │
        ├── gt mail send gastown/refinery -s "MERGE_READY: <branch>"
        │   └── Refinery processes merge queue
        │
        ├── gt mail send mayor/ -s "CONVOY COMPLETE: hq-abc"
        │   └── Mayor updates convoy tracking
        │
        └── gt mail send gastown/witness -s "POLECAT_DONE: <name>"
            └── (Self-mail for beads record)
```

### What Goes Where

| Communication | Channel | Why |
|--------------|---------|-----|
| Witness ↔ Polecat | AT messaging | Same team, real-time, ephemeral |
| Polecat ↔ Polecat | AT messaging | Same team, coordination chatter |
| Witness → Refinery | gt mail | Different lifecycle, needs persistence |
| Witness → Mayor | gt mail | Cross-rig, needs persistence |
| Mayor → Witness | gt mail | Cross-rig, needs persistence |
| Polecat escalation | AT message to Witness, Witness relays via gt mail | Bridge pattern |

### The Relay Pattern

Polecats can't send gt mail directly to entities outside their team (AT
messaging is team-scoped). Instead:

```
Polecat needs to escalate to Mayor:
    │
    ├── Polecat sends AT message to Witness:
    │   "ESCALATE: Need Mayor decision on auth approach"
    │
    └── Witness relays via gt mail:
        gt mail send mayor/ -s "ESCALATE from polecat <name>" -m "..."
```

This is analogous to the current model where polecats mail the Witness and
the Witness escalates. The difference: AT messaging is real-time (no Dolt
sync lag), and the Witness can relay immediately.

---

## 8. Configuration

### `.claude/settings.json` (Project Level)

```json
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  },
  "hooks": {
    "TaskCompleted": [{
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/task-completed-sync.sh"
      }]
    }],
    "TeammateIdle": [{
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/teammate-idle-check.sh"
      }]
    }],
    "SubagentStop": [{
      "matcher": "polecat",
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/teammate-stopped.sh"
      }]
    }]
  }
}
```

### `.claude/agents/witness-lead.md`

```yaml
---
name: witness-lead
description: Gas Town Witness operating as AT team lead
model: opus
permissionMode: delegate
hooks:
  SessionStart:
    - hooks:
        - type: command
          command: "export PATH=\"$HOME/go/bin:$HOME/.local/bin:$PATH\" && gt prime --hook"
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "gt witness-bash-guard"
  Stop:
    - hooks:
        - type: command
          command: "gt signal stop"
---

You are the Gas Town Witness for this rig.

## Role
You coordinate polecat workers. You NEVER implement code directly.
Delegate mode enforces this structurally — you cannot edit files.

## Startup
1. Check for incoming work: `gt mail inbox`, `bd ready`
2. Create AT team if work is available
3. Spawn polecat teammates for each issue
4. Monitor progress via AT task list

## During Work
- Monitor teammate progress via TaskList
- Relay cross-rig messages (gt mail ↔ AT messages)
- Handle teammate crashes (respawn or escalate)
- Enforce quality via plan approval

## Completion
- Verify all AT tasks completed
- Verify beads are synced (all issues closed)
- Send MERGE_READY to Refinery via gt mail
- Send convoy completion to Mayor via gt mail
- Shutdown team
```

### `.claude/agents/polecat.md`

See Section 2 above for the full definition.

---

## 9. What Gets Replaced

### Infrastructure Removed (Phase 1)

| Component | Replacement | Notes |
|-----------|-------------|-------|
| `gt sling` (polecat spawn) | `Teammate({ operation: "spawn" })` | AT native |
| `gt polecat nuke` | `Teammate({ operation: "requestShutdown" })` | AT native |
| tmux session management | AT manages teammate sessions | No more tmux for polecats |
| `gt nudge` (tmux send-keys) | `Teammate({ operation: "write" })` | AT messaging |
| Zombie detection (tmux-based) | SubagentStop / TeammateIdle hooks | Structural |
| Witness "are you stuck?" polling | TeammateIdle hook (automatic) | Event-driven |
| Polecat-to-polecat isolation | Prompt + PreToolUse hook | Behavioral → hook-enforced |

### Infrastructure Kept (Phase 1)

| Component | Why |
|-----------|-----|
| Beads (Dolt) | Durable ledger — AT tasks are ephemeral |
| gt mail | Cross-rig communication — AT is team-scoped |
| Molecules/formulas | Work templates — AT tasks created from these |
| `gt done` | Polecat self-clean — unchanged lifecycle |
| Git worktrees | Filesystem isolation — AT doesn't provide this |
| Daemon/Boot/Deacon | Health monitoring — AT has no crash recovery |
| Refinery (separate) | Different lifecycle (Phase 2 brings it in-band) |
| Convoy tracking | Cross-rig work orders — above AT scope |

### Dolt Write Pressure Reduction

**Current:** Every `bd update`, `bd close`, `bd create` from every polecat
= concurrent Dolt writes. 20 polecats = 20+ concurrent commits.

**With AT:** Real-time task coordination happens in AT (file-locked, no Dolt).
Dolt writes only at boundaries:
- `bd close` when a molecule step completes (1 per task)
- `bd create` when polecats discover new issues (rare)

**Estimated reduction: 80-90%.** The remaining writes are naturally staggered
across minutes (task completions), not milliseconds (concurrent status updates).

---

## 10. Witness Startup Flow (Updated)

```
Witness session starts (managed by daemon)
    │
    ├── SessionStart hook: gt prime --hook
    │   └── Loads role context, checks hook
    │
    ├── Check for work:
    │   ├── gt mail inbox (convoy dispatch, priority changes)
    │   ├── bd ready (unblocked issues)
    │   └── gt hook (hooked work)
    │
    ├── If work available:
    │   │
    │   ├── Create AT team:
    │   │   Teammate({ operation: "spawnTeam", team_name: "<rig>-work" })
    │   │
    │   ├── Create AT tasks from beads issues:
    │   │   For each issue: TaskCreate({ subject, description, metadata: { bead_id } })
    │   │   Set dependencies: TaskUpdate({ addBlockedBy: [...] })
    │   │
    │   ├── Create worktrees for polecats:
    │   │   For each polecat: git worktree add ...
    │   │
    │   ├── Spawn polecat teammates:
    │   │   For each (up to max_teammates):
    │   │     Task({ subagent_type: "polecat", team_name: "...", name: "..." })
    │   │
    │   └── Enter monitoring loop:
    │       ├── Watch AT task list for completions
    │       ├── Handle teammate crashes (SubagentStop)
    │       ├── Relay gt mail ↔ AT messages
    │       ├── Check for new convoy arrivals
    │       └── When all tasks done: cleanup and report
    │
    └── If no work:
        └── Stop hook checks for queued work periodically
            └── If work arrives: wake and create team
```

---

## 11. Phase 1 Scope and Validation Criteria

### In Scope

1. Witness as AT team lead in delegate mode (with Bash for gt/bd)
2. Polecat teammates with `.claude/agents/polecat.md`
3. Bead sync via TaskCompleted hook
4. Session cycling via PreCompact handoff + respawn
5. Basic error handling (crash detection, respawn, crash loop prevention)
6. Mail bridge (gt mail ↔ AT messaging)
7. Single-convoy sequential processing

### Out of Scope (Phase 2+)

1. Refinery as AT teammate
2. Multiple concurrent convoys
3. Cross-rig AT coordination
4. Crew squads / shadow workers
5. Advanced plan approval workflows
6. Performance optimization (token cost tuning)

### Validation Criteria

| Criterion | Test |
|-----------|------|
| Witness stays in delegate mode | Verify Witness cannot write/edit files |
| Polecats complete work | End-to-end: spawn → implement → push → gt done |
| Beads sync correctly | AT task completion → bd close fires → bead is closed |
| Session cycling works | Force compaction → new teammate resumes from beads |
| Crash recovery works | Kill a teammate → Witness detects → respawns |
| Mail bridge works | Mayor sends mail → Witness relays to polecat |
| Dolt writes reduced | Measure bd command frequency: before vs after |
| Token cost acceptable | `/cost` shows < 3x overhead vs current model |
| Convoy completes | Full convoy lifecycle: dispatch → work → merge → done |

---

## 12. Migration Path

### Current Architecture → Phase 1

The transition is additive: AT runs alongside existing infrastructure during
validation. The Witness can fall back to tmux-based management if AT fails.

```
Step 1: Enable AT feature flag in gastown .claude/settings.json
Step 2: Create .claude/agents/polecat.md and .claude/agents/witness-lead.md
Step 3: Implement hook scripts (task-completed-sync, teammate-idle, teammate-stopped)
Step 4: Implement gt witness-bash-guard
Step 5: Implement gt validate-worktree-scope
Step 6: Implement gt witness-teammate-stopped
Step 7: Update Witness startup to create AT team instead of tmux polecat sessions
Step 8: Test with 2 polecats on a small convoy
Step 9: Validate all criteria above
Step 10: If validated: expand to 3-5 polecats, larger convoys
```

### Rollback Plan

If Phase 1 fails:
1. Disable AT feature flag
2. Witness reverts to tmux-based polecat management
3. No beads data lost (beads sync is additive)
4. File lessons-learned bead for Phase 1 retry

---

*"The transport changes. The ledger endures."*
