# Refinery Merge Workflow

> How the Refinery processes completed polecat work and merges it to main

## Overview

The **Refinery** is Gas Town's merge queue processor - the Engineer in the engine room. It receives completed work from polecats, rebases branches sequentially on main, runs tests, and pushes merged changes. This document explains the complete merge workflow from polecat completion to main branch integration.

## Architecture Summary

The Refinery operates as:
- **One per rig** (e.g., `gastown/refinery`)
- **Persistent Claude agent** running in tmux
- **Agent-driven decisions** (ZFC #5) - the Claude agent makes all merge/conflict decisions, not Go code
- **Git worktree** based on `mayor/rig` (shares `.git` with mayor and polecats)
- **Sequential rebasing** - one branch at a time to prevent conflicts
- **Configurable merge strategy** - determines how work lands (direct merge vs PR)

## Merge Strategies

The Refinery supports four configurable merge strategies that determine how completed work lands after processing:

| Strategy | Description | Use Case |
|----------|-------------|----------|
| `direct_merge` | Merge directly to target branch (no PR) | Maintainer repos with direct push access |
| `pr_to_main` | Create GitHub PR targeting main | Repos with protected branches or review requirements |
| `pr_to_branch` | Create GitHub PR targeting a specific branch | Staging/develop branch workflows |
| `direct_to_branch` | Merge directly to a specific branch (no PR) | Staging branches without PR requirement |

### Configuring Merge Strategy

Strategy is configured per-rig in `config.json`:

```json
{
  "merge_queue": {
    "strategy": "direct_merge",
    "target_branch": "main",
    "pr_options": {
      "auto_merge": true,
      "labels": ["automated"],
      "reviewers": ["team-lead"]
    }
  }
}
```

**Configuration fields**:
- `strategy`: One of `direct_merge`, `pr_to_main`, `pr_to_branch`, `direct_to_branch`
- `target_branch`: Target branch for merges (default: `main`)
- `pr_options`: Settings for PR-based strategies (ignored for direct strategies)
  - `auto_merge`: Enable GitHub auto-merge when creating PR
  - `labels`: Labels to apply to created PRs
  - `reviewers`: GitHub usernames to request review from
  - `draft`: Create PR as draft

### Strategy Behavior

**Direct strategies** (`direct_merge`, `direct_to_branch`):
1. Rebase polecat branch onto target
2. Run tests
3. Fast-forward merge into target
4. Push to origin
5. Delete polecat branch
6. **Exit**: Work lands immediately on target

**PR strategies** (`pr_to_main`, `pr_to_branch`):
1. Rebase polecat branch onto target
2. Run tests locally
3. Push rebased branch to origin
4. Create GitHub PR via `gh pr create`
5. **Exit**: Work awaits external review/merge

For PR strategies, the MR bead tracks PR lifecycle:
- `pr_url`: GitHub PR URL
- `pr_number`: PR number
- `pr_state`: `open`, `merged`, or `closed`

### Checking Current Strategy

```bash
gt mq config <rig>  # Show merge queue configuration
```

## 1. How Refinery Detects Completed Work

### Source of Truth: Beads Merge Queue

The **beads merge queue** (`gt mq list <rig>`) is the ONLY source of truth for pending merges. The refinery NEVER uses `git branch -r | grep polecat` or `git ls-remote` to detect work.

**Why?** Beads MQ tracks:
- MR metadata (priority, worker, source issue)
- MR lifecycle (open, in_progress, closed)
- Blocking state (conflict resolution tasks)
- Claim state (for parallel refineries)

Git branches alone don't have this information.

### Detection Flow

```
Polecat completes work
     ↓
`gt done` creates MR bead (type: merge-request)
     ↓
Refinery patrol checks: `gt mq list <rig>`
     ↓
MR appears in queue with status: open
     ↓
Refinery processes next MR by priority
```

### Patrol Cycle

The refinery runs a patrol molecule (`mol-refinery-patrol`) that:

1. **inbox-check**: Process MERGE_READY mail from Witness (legacy protocol)
2. **queue-scan**: Check beads MQ for pending work
   ```bash
   git fetch --prune origin
   gt mq list <rig>
   ```
3. **process-branch**: Pick next MR and attempt merge
4. Loop until queue empty or context high

**Key insight**: The refinery is proactive. It doesn't wait for notifications - it polls the MQ and processes whatever is ready.

## 2. The Merge Queue Process

### Queue Storage

MRs are stored as JSON files in `.beads/mq/`:

```
.beads/mq/
├── mr-1234567890-abc123.json
├── mr-1234567891-def456.json
└── mr-1234567892-ghi789.json
```

These files are **ephemeral** - deleted after successful merge. They're NOT synced to git (unlike beads in `issues.jsonl`).

### MR Lifecycle States

| State | Description | Transitions |
|-------|-------------|-------------|
| `open` | Waiting to be processed | → in_progress (refinery claims) |
| `in_progress` | Currently being merged | → closed (success/rejection)<br>→ open (failure, needs rework) |
| `closed` | Completed (merged/rejected) | ❌ Immutable once closed |

### Priority Scoring

MRs are scored for priority using `mrqueue.ScoreMR`:

```go
score = base_priority_points
      - (retry_count * retry_penalty)  // Prevent thrashing
      + convoy_age_bonus                // Prevent starvation
      + mr_age_tiebreaker               // FIFO for same priority
```

**Factors**:
- **Priority level** (P0-P4): Lower number = higher priority
- **Retry count**: Each conflict retry reduces priority to prevent thrashing
- **Convoy age**: Old convoys get boosted to prevent starvation
- **MR age**: Older MRs break ties for same priority

### Blocking and Delegation

MRs can be **blocked** by tasks (e.g., conflict resolution):

```
MR has conflict
     ↓
Refinery creates conflict-resolution task
     ↓
MR.BlockedBy = task_id
     ↓
MR stays in queue but is skipped
     ↓
When task closes, MR unblocks and becomes ready
```

This enables **non-blocking delegation** - the refinery doesn't wait for conflict resolution, it continues to the next MR.

## 3. Interaction with Polecat Worktrees

### Worktree Architecture

```
<rig>/
├── .repo.git/           Bare repo (shared by all worktrees)
├── mayor/rig/           Canonical clone (.beads lives here)
├── refinery/rig/        Worktree on main branch
└── polecats/<name>/     Polecat worktrees (ephemeral)
```

All worktrees share the **same `.git` object database** via `.repo.git`. This means:
- ✅ Commits are immediately visible across worktrees
- ✅ No fetching needed for local branches
- ✅ Fast worktree spawning
- ⚠️ Branches must have unique names (enforced by polecat naming)

### Refinery's Git Context

The refinery operates in `refinery/rig/`:
- **Working directory**: `<rig>/refinery/rig/`
- **Current branch**: Usually `main`
- **Shared git**: Via `.repo.git` (sees all polecat branches locally)

### Branch Interaction

When processing a polecat branch:

```bash
# Branch exists locally (shared .git)
git branch -a | grep polecat/nux
# → polecat/nux  (local branch, visible to refinery)

# Refinery checks it out directly
git checkout -b temp polecat/nux
# No fetch needed - branch is already local!

# After merge, refinery deletes both local and remote
git branch -d polecat/nux           # Local
git push origin --delete polecat/nux  # Remote
```

**Key insight**: The refinery doesn't "pull" polecat work. Polecat branches are already local in the shared git database.

### After Polecat Dies

When a polecat runs `gt done`:
1. Work is pushed to `origin/polecat/<name>`
2. MR bead created in `.beads/mq/`
3. Polecat session exits
4. Polecat worktree is nuked (self-cleaning model)

The **branch lives on in git** (both locally in `.repo.git` and on `origin`). The refinery processes the branch even though the polecat is gone.

## 4. What Triggers a Merge to Main

### Automatic Processing

Merges are **fully automatic**. There is no approval gate. When a polecat completes work:

1. MR enters the queue (status: open)
2. Refinery picks it up on next patrol cycle
3. Refinery attempts to merge immediately

**No human approval needed.** The gate is that the polecat completed the work successfully.

### Pre-Merge Checks

Before merging, the refinery verifies:

1. **Branch exists**: `git branch -a | grep <branch>`
2. **No merge conflicts**: `git rebase origin/main` succeeds
3. **Tests pass** (if configured): `go test ./...` or custom test command
4. **handle-failures gate**: If tests fail, refinery MUST:
   - Fix it (refinery can code!)
   - OR file a bug bead for tracking
   - Cannot proceed without one of these

### Sequential Rebasing Protocol

```
WRONG (parallel merge - causes conflicts):
  main ─────────────────────────────┐
    ├── branch-A (based on old main) ├── CONFLICTS
    └── branch-B (based on old main) │

RIGHT (sequential rebase):
  main ──────┬────────┬─────▶ (clean history)
             │        │
        merge A   merge B
             │        │
        A rebased  B rebased
        on main    on main+A
```

After every successful merge:
1. Main moves forward
2. Next branch rebases on **new main**
3. This prevents conflicts and keeps history linear

## 5. Auto-Merge vs. Manual Approval

### Gas Town Uses Auto-Merge

The refinery **auto-merges** all work. There is NO manual approval step.

**Why?** Gas Town is designed for autonomous agent operation:
- Polecats complete molecules with defined acceptance criteria
- Work is verified before submission (`gt done` checks git state)
- Tests run before merge (the actual gate)
- Issues tracked in beads for audit trail

**The approval was when the work was slung.** Slinging work implies:
- The issue is well-defined
- The work should be done
- The result should land on main

### Conflict "Approval" (Indirect)

The refinery doesn't approve MRs, but it does make **conflict decisions**:

| Scenario | Decision | Approval? |
|----------|----------|-----------|
| Clean merge | Auto-merge | ❌ No approval |
| Trivial conflict | Resolve and merge | ✅ Agent judgment |
| Complex conflict | Delegate to conflict task | ✅ Agent judgment |
| Tests fail (branch) | Reject, notify polecat | ✅ Agent judgment |
| Tests fail (pre-existing) | Fix or file bug | ✅ Agent judgment |

The refinery agent **makes decisions** (ZFC #5), but doesn't "approve" in the pull-request sense.

## 6. Conflict Handling

### Detection

Conflicts are detected during rebase:

```bash
git checkout -b temp polecat/<worker>
git rebase origin/main
# Exit code 1 + .git/rebase-merge exists = CONFLICT
```

### Resolution Strategies

The refinery has two options:

#### Option A: Resolve Mechanically (Trivial Conflicts)

If conflicts are simple (whitespace, imports, etc.):

```bash
# Edit conflicted files
vim conflicted_file.go
# Stage resolution
git add conflicted_file.go
# Continue rebase
git rebase --continue
# Proceed to merge
```

**Judgment call**: The refinery agent decides if resolution is safe.

#### Option B: Delegate (Complex Conflicts)

If conflicts are complex:

1. **Abort rebase**:
   ```bash
   git rebase --abort
   ```

2. **Create conflict-resolution task**:
   ```bash
   bd create --type=task --priority=1 \
     --title="Resolve merge conflicts: <original-issue>"
   ```

3. **Block the MR**:
   ```json
   {
     "id": "mr-abc",
     "blocked_by": "gt-conflict-xyz"
   }
   ```

4. **Continue to next MR**: Non-blocking delegation!

### Merge Slot Serialization

To prevent **multiple simultaneous conflict resolutions**, the refinery uses a **merge slot**:

```
Before creating conflict task:
    ↓
Check if merge slot available
    ↓
If held by another: defer (skip task creation)
If available: acquire slot, create task
    ↓
Task blocks MR until resolved
    ↓
After successful merge: release slot
```

**Why?** Prevents multiple polecats from resolving conflicts on different branches simultaneously, which can lead to cascading conflicts.

### Conflict Task Format

```markdown
## Conflict Resolution Required

Original MR: mr-1234567890-abc
Branch: polecat/nux
Original Issue: gt-work-123
Conflict with main at: a1b2c3d4

## Instructions
1. Clone/checkout the branch
2. Rebase on current main: git rebase origin/main
3. Resolve conflicts
4. Force push: git push -f origin polecat/nux
5. Close this task when done

The MR will be re-queued for processing after conflicts are resolved.
```

The task is dispatchable via `bd ready` - any available polecat can pick it up.

### After Resolution

When the conflict task closes:
1. MR's `blocked_by` becomes empty
2. MR re-enters ready queue
3. Refinery retries on next patrol
4. If clean this time → merge succeeds

## 7. Post-Merge Workflow

### Immediate Actions (Critical!)

The refinery template enforces a **strict post-merge sequence**:

```bash
# Step 1: Merge and push
git checkout main
git merge --ff-only temp
git push origin main

# ⚠️ STOP - DO STEPS 2-3 BEFORE CLEANUP

# Step 2: Send MERGED notification (REQUIRED)
gt mail send <rig>/witness -s "MERGED <polecat-name>" \
  -m "Branch: <branch>
Issue: <issue-id>
Merged-At: $(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Step 3: Close MR bead (REQUIRED)
bd close <mr-bead-id> --reason "Merged to main"

# Step 4: Archive MERGE_READY mail (REQUIRED)
gt mail archive <message-id>

# Step 5: Cleanup (only after 2-4 confirmed)
git branch -d temp
git push origin --delete <polecat-branch>
```

**Why this order?**

1. **Push first**: Work is safely on main before any notifications
2. **Notify immediately**: Witness needs to know ASAP to nuke polecat worktree
3. **Close MR before cleanup**: Audit trail that work was merged
4. **Archive mail**: Inbox hygiene

**Failure mode prevented**: Without MERGED notification, polecat worktrees accumulate indefinitely and the lifecycle breaks.

### Beads Cleanup

After merge, the refinery:

1. **Closes MR bead**:
   ```bash
   bd close <mr-id> --reason "Merged to main at <sha>"
   ```

2. **Closes source issue** (if specified):
   ```bash
   bd close <source-issue-id> --reason "Merged in <mr-id>"
   ```

3. **Clears agent bead reference**:
   ```go
   beads.UpdateAgentActiveMR(agentBead, "") // Traceability cleanup
   ```

4. **Removes MR from queue**:
   ```bash
   rm .beads/mq/<mr-id>.json
   ```

### Branch Cleanup

If configured (`delete_merged_branches: true`):

```bash
# Delete local polecat branch
git branch -d polecat/<worker>

# Delete remote polecat branch
git push origin --delete polecat/<worker>
```

**Why delete remote?** As of the self-cleaning model (Jan 2025), polecats push to origin before `gt done`, so branches exist on both local and remote.

### Event Logging

The refinery logs events to the feed:

```go
events.LogFeed(events.TypeMerged, actor, MergePayload{
    MR: mr.ID,
    Worker: mr.Worker,
    Branch: mr.Branch,
    Commit: mergeCommit,
})
```

These appear in `gt feed` and `gt activity` for monitoring.

## 8. Change Syncing Back to Polecats

### The Key Insight: Polecats Are Gone

**There is no syncing back to polecats.** By the time the refinery merges work:

1. The polecat has already run `gt done`
2. The polecat session has exited
3. The polecat worktree has been nuked (self-cleaning)
4. The polecat slot is released back to the pool

**The polecat no longer exists.** There's nothing to sync back to.

### What If There's a Conflict?

If a conflict occurs:

1. Refinery creates a **conflict-resolution task**
2. Task is dispatchable (any polecat can pick it up)
3. A **FRESH polecat** is spawned to resolve it
4. Fresh polecat does the work and re-submits

**Never send work back to the original polecat - it's gone.**

This is the **self-cleaning polecat model**:
- Polecats don't wait around for merge results
- Work lives in the MQ, not in the polecat
- If rework is needed, spawn a fresh polecat

### Crew Workspace Syncing

**Crew members** (humans) DO need to sync:

```bash
# In crew/<name>/
git fetch origin
git pull origin main
```

Crew workspaces are **persistent full clones**. They stay around between work sessions and need manual syncing.

But polecats don't - they're ephemeral and don't survive past work completion.

### Shared .git Model

All worktrees share `.repo.git`, so:

- When refinery pushes to `origin/main`
- Other worktrees see it immediately in shared git database
- No explicit "sync" needed for local visibility

But the **working directory** needs updating:

```bash
# In mayor/rig/ or refinery/rig/
git checkout main
git pull origin main  # Updates working directory
```

Polecats don't do this because they're already gone.

## Summary Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                  Polecat Completes Work                     │
│  → `gt done` creates MR bead                                │
│  → Pushes branch to origin                                  │
│  → Exits session (self-cleaning)                            │
│  → Worktree nuked                                           │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│              Refinery Patrol Cycle (Proactive)              │
│  1. inbox-check: Process MERGE_READY mail                  │
│  2. queue-scan: `gt mq list <rig>` (source of truth)       │
│  3. Found MR in queue (status: open)                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  process-branch (Sequential)                │
│  → git checkout -b temp polecat/<worker>                   │
│  → git rebase origin/main                                  │
│                                                             │
│  If conflict:                                              │
│    → Create conflict task                                  │
│    → Block MR on task                                      │
│    → Continue to next MR (non-blocking)                    │
│                                                             │
│  If clean:                                                 │
│    → Continue to run-tests                                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    run-tests (If Configured)                │
│  → go test ./... (or custom command)                       │
│                                                             │
│  If tests fail:                                            │
│    → Diagnose: branch regression or pre-existing?          │
│    → Fix it OR file bug bead (GATE: cannot proceed)        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│               merge-push (Critical Sequence)                │
│  1. git merge --ff-only temp && git push origin main       │
│  2. Send MERGED mail to witness (REQUIRED)                 │
│  3. Close MR bead (REQUIRED)                               │
│  4. Archive MERGE_READY mail (REQUIRED)                    │
│  5. Cleanup branches                                       │
│                                                             │
│  Main has moved → next branch rebases on new baseline      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   loop-check & Continue                     │
│  More branches? → Return to process-branch                 │
│  Queue empty? → Generate summary, check context            │
│  High context? → gt handoff for fresh session              │
└─────────────────────────────────────────────────────────────┘
```

## Key Principles

1. **Beads MQ is source of truth** - Never rely on git branches alone
2. **Sequential rebasing** - One branch at a time prevents conflicts
3. **Agent-driven decisions** (ZFC #5) - Claude agent makes all merge/conflict decisions
4. **Auto-merge, no approval** - Work is automatically merged when clean
5. **Shared .git architecture** - All worktrees see commits immediately
6. **Self-cleaning polecats** - Polecats don't exist after completion, no syncing back
7. **Non-blocking delegation** - Conflicts don't stop the queue
8. **Strict post-merge sequence** - Notification before cleanup prevents lifecycle breaks

## Related Documentation

- [Polecat Lifecycle](polecat-lifecycle.md) - Self-cleaning model and session cycles
- [Architecture](../design/architecture.md) - Worktree architecture and beads routing
- [Molecules](molecules.md) - Patrol molecule execution
- [Propulsion Principle](propulsion-principle.md) - Why work triggers immediate execution
- Refinery role template: `internal/templates/roles/refinery.md.tmpl`
- Patrol formula: `.beads/formulas/mol-refinery-patrol.formula.toml`
