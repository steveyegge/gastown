# E2E Integration Branch Tests: Investigation & Design

> **Context**: PR #1226 (Xexr) adds integration branch support to Gas Town.
> Julian's [manual test plan](https://gist.github.com/julianknutsen/04217bd547d382ce5c6e37f44d3700bf) covers 28 tests across 6 parts.
> Xexr's [adapted version](https://gist.github.com/Xexr/f6c6902fe818aad7ccdf645d21e08a49) accounts for fork environment differences.
> This document investigates how to automate those tests as CI/CD integration tests.

---

## 1. What We're Testing

The integration branch lifecycle is a multi-component pipeline:

```
gt mq integration create <epic>     → creates branch off main, pushes to origin
gt sling <bead> <rig>               → dispatches work to polecat
gt done                              → polecat submits MR (target: integration branch)
Refinery (Claude + formula)          → merges polecat branch → integration branch
gt mq integration land <epic>        → merges integration branch → main (with guardrails)
```

Julian's test plan has 28 tests across 6 parts:
- **Part A** (A1-A3): Unit tests — config parsing, branch naming, target detection
- **Part B** (B1-B3): Regression — pre-closed epic, stale ref, duplicate create
- **Part C** (C1-C8): Lifecycle — create → sling → MR → merge → land (the hard part)
- **Part D** (D1-D4): Config & overrides — template overrides, `auto_land=false`
- **Part E** (E1-E5): Edge cases — conflicts, concurrent MRs, force push
- **Part F** (F1-F3): Deprecation — doctor checks, settings migration

---

## 2. Production Architecture: How the Refinery Actually Works

### The production stack (5 layers)

In production, the refinery merge queue works through a 5-layer stack:

```
Layer 1: Daemon (daemon.go)
  └─ Heartbeat loop calls ensureRefineryRunning() every 3 min
     └─ Creates refinery.Manager, calls mgr.Start()

Layer 2: Refinery Manager (manager.go)
  └─ Creates tmux session "gt-<rig>-refinery"
     └─ Working dir: <rig>/refinery/rig/
        └─ Starts Claude Code with initial prompt: "Run gt prime --hook and begin patrol."

Layer 3: Claude Code (LLM instance)
  └─ SessionStart hook runs "gt prime --hook"
     └─ Detects role: refinery
        └─ Outputs formula context to Claude's conversation

Layer 4: Formula (mol-refinery-patrol.formula.toml)
  └─ Step-by-step instructions Claude follows:
     inbox-check → queue-scan → process-branch → run-tests →
     handle-failures → merge-push → loop-check → ...

Layer 5: Shell commands (what Claude actually executes)
  └─ git fetch --prune origin
  └─ gt refinery ready <rig>          ← uses Engineer.ListReadyMRs()
  └─ git checkout -b temp origin/<polecat-branch>
  └─ git rebase origin/main
  └─ git merge --ff-only temp
  └─ git push origin main
  └─ bd close <mr-bead-id>
  └─ gt mail send <rig>/witness -s "MERGED <polecat>"
```

The merge itself — the rebase, merge, and push — is performed by **Claude executing raw git commands**, following the formula's step-by-step instructions. The formula IS the control plane.

### The Engineer struct: two roles, only one used in production

The `Engineer` struct (`internal/refinery/engineer.go`) contains two categories of methods:

**Query/state methods — USED in production** (called by CLI commands that the formula tells Claude to run):

| Method | Called by | What it does |
|--------|----------|-------------|
| `ListReadyMRs()` | `gt refinery ready` | Lists unclaimed, unblocked MRs |
| `ListBlockedMRs()` | `gt refinery blocked` | Lists MRs blocked by open tasks |
| `ListQueueAnomalies()` | `gt refinery ready` | Detects stale claims, orphaned branches |
| `ClaimMR()` | `gt refinery claim` | Assigns MR to a worker |
| `ReleaseMR()` | `gt refinery release` | Returns MR to queue |

**Merge execution methods — ZERO callers anywhere in the codebase:**

| Method | Callers | Status |
|--------|---------|--------|
| `ProcessMR()` | None | Dead code |
| `ProcessMRInfo()` | None | Dead code |
| `doMerge()` | Only by ProcessMR/ProcessMRInfo (internal) | Dead code |
| `handleSuccess()` | None | Dead code |
| `HandleMRInfoSuccess()` | None | Dead code |
| `HandleMRInfoFailure()` | None | Dead code |

There is no `gt refinery process` or `gt refinery merge` command. No CLI command calls any of the merge execution methods. The formula tells Claude to run git commands directly — it never invokes the Engineer's merge logic.

### Why the merge methods exist but aren't used

The Engineer's merge methods appear to be a **Go library API** that encapsulates the same merge logic as the formula's git commands:

```go
// engineer.go — doMerge() mirrors the formula's process-branch + merge-push steps
func (e *Engineer) doMerge(ctx, branch, target, sourceIssue) ProcessResult {
    e.git.BranchExists(branch)           // formula: git branch -r | grep <branch>
    e.git.Checkout(target)               // formula: git checkout main
    e.git.Pull("origin", target)         // formula: (implicit in git fetch)
    e.git.CheckConflicts(branch, target) // formula: git rebase detects conflicts
    e.git.MergeSquash(branch, msg)       // formula: git merge --ff-only temp
    e.git.Push("origin", target)         // formula: git push origin main
}
```

But these methods are never wired into any command or production path. The production refinery relies entirely on Claude following formula instructions. The Go library API was likely either:
- Built as a programmatic foundation that was superseded by the formula-driven approach
- Intended for a future `gt refinery process` command that was never created
- A testable encapsulation of merge logic that predates the current formula architecture

**The formula and the Engineer's merge methods are NOT "two parallel implementations" — the formula is the only implementation used in production. The Engineer's merge methods are unused code.**

### Implications for e2e testing

This is a critical finding for test design. We have three options for testing the merge step:

| Approach | What it tests | Fidelity |
|----------|-------------|----------|
| **A: Call `Engineer.ProcessMRInfo()` directly** | The Go merge logic (squash merge via `e.git.*`) | Tests code that is never called in production |
| **B: Simulate what Claude does (raw git commands)** | The actual production merge path | Tests what actually runs, but just tests git itself |
| **C: Test the CLI commands the formula references** | `gt refinery ready`, `gt refinery claim`, etc. | Tests the query/state layer that IS used in production |

**Option A** (what the original investigation proposed) tests the unused Go library path. While this validates the merge mechanics work correctly, it's testing dead code — if the merge methods had a bug, production wouldn't be affected because production doesn't call them.

**Option B** would essentially be testing `git rebase` and `git merge` — we'd be testing git, not our code.

**Option C** tests the parts of the Engineer that production actually uses: queue listing, MR claiming, anomaly detection.

**Recommended approach for Part C lifecycle tests:**
- Use **subprocess calls** for the entire lifecycle: `gt mq integration create`, simulate polecat work with git commands, simulate the merge with git commands (mimicking what Claude does), then `gt mq integration land`
- Use **Option C** to test that `gt refinery ready` correctly lists MRs targeting integration branches
- Reserve **Option A** only if the merge methods are ever wired into a production command

### What we're really testing (revised)

The e2e tests for integration branches should focus on what our code actually does:

| What we're testing | How | Our code involved |
|-------------------|-----|-------------------|
| Branch creation | `gt mq integration create` subprocess | `runMqIntegrationCreate()` |
| Target detection | `gt done` subprocess or `detectIntegrationBranch()` | `mq_submit.go` |
| MR listing for integration branch | `gt refinery ready` subprocess | `Engineer.ListReadyMRs()` |
| Integration status reporting | `gt mq integration status` subprocess | `runMqIntegrationStatus()` |
| Landing with guardrails | `gt mq integration land` subprocess | `runMqIntegrationLand()` |
| The merge itself | Raw git commands in test (simulating Claude/formula) | None — git does this, not our code |

The merge step in production is Claude running `git rebase` + `git merge --ff-only` + `git push`. Our code doesn't participate in the merge execution — it only sets up the branches (create), detects the target (done/submit), lists the queue (refinery ready), and tears down afterward (land). **Those are the boundaries we should test.**

---

## 3. Formula vs Engineer: Divergences and LLM-Dependent Weak Points

### Merge strategy divergence

The formula and the Engineer's unused merge methods implement **different merge strategies**:

| | Formula (production) | Engineer (unused) |
|---|---|---|
| **Strategy** | `rebase` + `merge --ff-only` | `merge --squash` |
| **History** | Linear — original commits preserved | Single squash commit |
| **Conflict detection** | Side effect of rebase attempt | Separate pre-flight `CheckConflicts()` (test merge with `--no-commit --no-ff`, inspect, abort) |
| **Branch handling** | Creates `temp` branch from `origin/<branch>` | Works with local branch directly |
| **Target update** | Implicit via `git fetch` | Explicit `e.git.Pull("origin", target)` |

These produce **different commit histories** on the same inputs.

### Conflict resolution lifecycle divergence

When a merge conflict is detected, the two paths diverge significantly:

**Formula (production):**
1. Rebase fails → `git rebase --abort`
2. Claude runs `bd create --type=task ...` to create a conflict resolution task
3. Skip MR, move to next branch
4. **No dependency created** between MR and conflict task
5. Next patrol cycle → `gt mq list` shows MR still "open" → Claude tries rebase again
6. If branch was force-pushed with resolution → rebase succeeds
7. If not → Claude hits same conflict, potentially creates **duplicate** conflict task

**Engineer (unused):**
1. `doMerge` returns `Conflict: true`
2. `HandleMRInfoFailure()` creates conflict task AND calls `e.beads.AddDependency(mr.ID, taskID)` — **blocks the MR on the task**
3. Merge slot acquired — serializes conflict resolution (only one at a time)
4. `ListReadyMRs()` filters out blocked MRs — refinery won't retry until task is closed
5. When someone closes the task → MR unblocks → appears in `gt refinery ready` again

| Behavior | Formula | Engineer |
|----------|---------|----------|
| MR blocked until resolution? | No — retries every patrol cycle | Yes — blocked via beads dependency |
| Duplicate conflict tasks? | Possible on each retry | No — MR is blocked, won't be reprocessed |
| Serialized resolution? | No | Yes — merge slot |
| Retry trigger | Next patrol cycle (time-based) | Task closure (event-based) |

### Who resolves the conflict task?

A dedicated **conflict resolution formula** exists: `mol-polecat-conflict-resolve.formula.toml`. It guides a polecat through conflict resolution with a fundamentally different merge path than normal polecat work:

| Aspect | Regular polecat work | Conflict resolution |
|--------|---------------------|---------------------|
| Branch | Create new branch | Checkout existing MR branch |
| Merge path | Submit to queue via `gt done` | **Push directly to target branch** |
| Issue closure | Refinery closes after merge | Polecat closes MR bead itself |
| Serialization | None | Merge-slot gate required |
| Formula | `mol-polecat-work` (auto-applied) | `mol-polecat-conflict-resolve` (must be explicit) |

The conflict resolution formula's steps:
1. **load-task** — parse metadata (original MR, branch, conflict SHA) from task description
2. **acquire-slot** — `bd merge-slot acquire --wait` (serializes resolution)
3. **checkout-branch** — `git checkout -b temp-resolve origin/<branch>`
4. **rebase-resolve** — `git rebase origin/main`, resolve conflicts using judgment
5. **run-tests** — verify resolution doesn't break anything
6. **push-to-main** — `git push origin temp-resolve:<target>` (**bypasses merge queue**)
7. **close-beads** — close original MR bead AND source issue (refinery normally does this)
8. **release-slot** — `bd merge-slot release`
9. **cleanup-and-exit** — close conflict task, `gt done`

**Why direct push?** Going back through the merge queue would create an infinite loop — the MR was already reviewed/approved, the polecat is just resolving conflicts.

### Dispatch gap: nobody auto-slings conflict tasks

The conflict task is created as a regular bead (`type: task`) and appears in `bd ready`. However:

1. **The witness does NOT scan `bd ready`** for unassigned tasks. It only reacts to mail (MERGE_READY, POLECAT_DONE, etc.). The witness patrol formula has no step for proactive task dispatch.

2. **`gt sling` auto-applies `mol-polecat-work`** (line 387-389 of `sling.go`), NOT `mol-polecat-conflict-resolve`. To use the correct formula, you must explicitly specify it:
   ```bash
   gt sling mol-polecat-conflict-resolve --on <conflict-task-id> gastown
   ```

3. **No agent proactively monitors `bd ready`** and dispatches conflict tasks.

The task sits in `bd ready` until someone (human or agent) manually runs the sling command with the correct formula. **This is a manual gap in an otherwise automated pipeline.**

### MERGE_FAILED protocol: notification only, no dispatch

The formula's `handle-failures` step sends `MERGE_FAILED` to the witness for **test failures** (not conflicts). For conflicts, the formula just creates a task and skips — no MERGE_FAILED is sent.

The witness's `HandleMergeFailed()` handler (`handlers.go:368-416`) only notifies the original polecat that their merge was rejected. It does NOT:
- Create tasks
- Dispatch work
- Trigger conflict resolution

And the original polecat may already be nuked after `gt done`, so the notification may go nowhere.

### End-to-end conflict resolution flow (actual)

```
Refinery patrol: rebase fails
  ↓
Formula: Claude creates conflict task (bd create --type=task ...)
  ↓
Task appears in `bd ready`
  ↓
 ── MANUAL GAP ──
  ↓
Human/agent runs: gt sling mol-polecat-conflict-resolve --on <task> <rig>
  ↓
Polecat spawned with conflict resolution formula
  ↓
Polecat: acquires merge slot
  ↓
Polecat: git checkout + rebase + resolve conflicts (LLM judgment)
  ↓
Polecat: runs tests
  ↓
Polecat: pushes DIRECTLY to target branch (bypasses merge queue)
  ↓
Polecat: closes MR bead + source issue + conflict task
  ↓
Polecat: releases merge slot, runs gt done
  ↓
Witness: receives POLECAT_DONE, nukes polecat
```

**This is an untested lifecycle with a manual dispatch gap and no deterministic guarantee of completion.**

### LLM-dependent weak points in the formula

Every step in the formula is prose that Claude must follow correctly. With different LLM models configured for refineries, behavior becomes uncertain at these points:

| Formula Step | What Claude must do | Failure mode with weaker model |
|---|---|---|
| **inbox-check** | Parse MERGE_READY mail, extract and **remember** branch, issue, polecat name, MR bead ID across multiple later steps | Forgets polecat name → MERGED notification fails → polecat worktrees accumulate indefinitely |
| **process-branch** | Substitute correct branch names into git commands, detect conflict state from exit codes | Wrong branch name → merges wrong code or loses work |
| **handle-failures** | Diagnose whether test failure is a branch regression vs pre-existing on target branch | Wrong diagnosis → merges broken code OR rejects good code |
| **merge-push** | Verify SHA match after push, send MERGED mail with correct polecat name, close correct MR bead, archive correct message — all in sequence | Any dropped step → silent lifecycle breakage (orphaned worktrees, orphaned beads, inbox bloat) |
| **check-integration-branches** | Read `auto_land` config value, respect FORBIDDEN directive | Ignores FORBIDDEN → lands integration branch autonomously when it shouldn't |
| **conflict handling** | Create properly formatted conflict task with all metadata fields | Missing metadata → conflict task is useless. Duplicate tasks on retry cycles. |

**The pre-push hook is the ONE deterministic guardrail** — it's code that runs regardless of what the LLM does. Everything else in the merge pipeline is prose-dependent.

### Where deterministic Go code could replace LLM prose

The Engineer's unused merge methods already implement most of the merge-push sequence deterministically. A command like `gt refinery process-next` that wires in the Engineer's merge logic would make the critical path code-deterministic:

| Current (LLM-dependent) | Potential (deterministic) |
|---|---|
| Claude remembers polecat name across steps | Go struct carries `MRInfo` through the pipeline |
| Claude constructs git commands from prose | `doMerge()` calls `e.git.*` methods |
| Claude decides whether to send MERGED mail | `HandleMRInfoSuccess()` always sends notification |
| Claude creates conflict task from prose template | `createConflictResolutionTaskForMR()` with dependency blocking |
| Claude diagnoses test failure cause | Could be codified: run tests on target first, then on merge, compare |

The LLM's role would shrink from **executing the merge mechanics** to **orchestrating the patrol loop** (when to process, whether to continue or hand off) — a much smaller surface area for model-dependent behavior.

### Implications for e2e testing

These findings affect what we can and should test:

1. **Formula FORBIDDEN directives are untestable in Go** — they're Claude-level guardrails. The pre-push hook is the testable enforcement layer.
2. **Conflict resolution lifecycle is untested end-to-end** — no test covers: conflict detected → task created → task dispatched → conflict resolved → direct push → beads closed.
3. **The formula's retry-without-blocking approach risks duplicate conflict tasks** — this is a testable scenario (the Engineer's dependency-blocking approach prevents this, but is unused).
4. **`gt sling` formula routing is not conflict-aware** — auto-applies `mol-polecat-work` instead of `mol-polecat-conflict-resolve`. This could be tested: sling a conflict task to a polecat, verify the wrong formula is applied.
5. **Direct push bypasses merge queue** — the conflict resolution formula pushes directly to the target branch. This is intentional (avoids infinite loop) but means the merge queue's test/validation step is skipped for resolved conflicts. Testable: verify push goes to correct target branch.
6. **If `gt refinery process-next` existed**, the merge pipeline + conflict handling would be deterministically testable without simulating Claude. The Engineer already has dependency-blocking and merge-slot serialization — the formula has neither.

---

## 4. Deep Investigation: Conflict Resolution Pipeline Gaps

Section 3 identified the dispatch gap. This section reports a deeper investigation that uncovered **six interconnected gaps** in the conflict resolution pipeline — and confirmed that the entire pathway has **never been exercised in production**.

### 4.1 Zero conflict tasks have ever been created

A comprehensive search of the gastown beads database found:

| Query | Results |
|-------|---------|
| `bd search "Resolve merge"` | No issues found |
| `bd search "merge conflict"` | No issues found |
| `bd list --type=merge-request --json` | `[]` (empty) |
| `bd list --label=gt:merge-request --json` | `[]` (empty) |
| `gt mq list gastown` | (empty) |

No conflict resolution tasks have ever been created on this rig. No MR beads exist at all. **The entire conflict resolution pipeline is untested in practice — not just the dispatch, but the creation itself.**

### 4.2 Format incompatibility between creation and consumption

The `mol-polecat-conflict-resolve` formula expects structured metadata in the task description:

```
## Metadata
- Original MR: <mr-id>
- Branch: <branch>
- Conflict with: <target>@<main-sha>
- Original issue: <source-issue>
- Retry count: <count>
```

But the refinery patrol formula (the only production path) creates a **different format**:

```
## Conflict Resolution Required

Original MR: <mr-bead-id>
Branch: <polecat-branch>
Original Issue: <issue-id>
Conflict with main at: ${MAIN_SHA}
Branch SHA: ${BRANCH_SHA}
```

| Field | Conflict-resolve expects | Formula creates |
|-------|-------------------------|-----------------|
| Header | `## Metadata` | `## Conflict Resolution Required` |
| MR field | `- Original MR: <id>` | `Original MR: <id>` (no list marker) |
| SHA field | `- Conflict with: <target>@<sha>` | `Conflict with main at: <sha>` (different format) |
| Retry count | `- Retry count: <N>` | Not included |
| Branch field | `- Branch: <branch>` | `Branch: <branch>` (no list marker) |

The conflict-resolve formula's load-task step tells the polecat to parse `## Metadata` with `- Field: value` syntax. The refinery formula creates prose without list markers and a different header. **Two different authors created incompatible formats.** A polecat following the conflict-resolve formula would fail to extract the metadata it needs.

The Engineer's `createConflictResolutionTaskForMR()` (dead code) produces the structured `## Metadata` format that the conflict-resolve formula expects — but it's never called.

### 4.3 Infinite retry loop / duplicate task creation

The formula path **does not block the MR** on the conflict task. The Engineer's dead code calls `e.beads.AddDependency(mr.ID, taskID)` — the formula does not.

Consequence on the next patrol cycle:
1. Claude runs `gt mq list` → sees the MR still open (no blocker)
2. Tries rebase again → hits the same conflict
3. Creates **another** conflict resolution task
4. Repeat every patrol cycle (default 30s interval)

After 10 patrol cycles: 10 identical conflict tasks in `bd ready`, the MR still stuck, no resolution dispatched. The Engineer's dependency-blocking prevents this entirely — blocked MRs don't appear in `ListReadyMRs()`.

### 4.4 No agent auto-dispatches conflict tasks

Verified across every agent role in the system:

| Agent | Role | Scans `bd ready`? | Dispatches conflict tasks? |
|-------|------|-------------------|---------------------------|
| **Witness** | Polecat monitor | No — reacts to mail only | No |
| **Deacon** | Infrastructure health | No — dispatches gated molecules and convoy dogs | No |
| **Boot** | Deacon watchdog | No — monitors Deacon health only | No |
| **Refinery** | Merge processor | Creates task, stops there | No `gt sling` call |

The deacon's `dispatch-gated-molecules` step dispatches gated molecules and stranded convoys. It has no awareness of conflict tasks. The witness's patrol only handles inbox mail (POLECAT_DONE, MERGE_FAILED, HELP). **No agent monitors `bd ready` for conflict tasks.**

### 4.5 MERGE_FAILED protocol is silent on conflicts

The refinery formula sends `MERGE_FAILED` mail to the witness **only for test/build failures**:

```
gt mail send <rig>/witness -s "MERGE_FAILED <polecat>" -m "...FailureType: quality-check..."
```

For **conflicts**, the formula just creates a task and skips — no mail is sent. The witness is never notified that a conflict occurred. This means:
- No mail-based trigger exists for conflict resolution
- The conflict task is created silently
- Even if the witness had a conflict-dispatch handler, it would never fire because no conflict mail arrives

### 4.6 Merge slot only exists in dead code

The `mol-polecat-conflict-resolve` formula's step 2 (acquire-slot) tells the polecat to run `bd merge-slot acquire --wait`. This serializes resolution so only one polecat resolves conflicts at a time.

But the merge slot **setup** happens in `createConflictResolutionTaskForMR()` (Engineer, dead code):

```go
// engineer.go:702-724
slotID, err := e.beads.MergeSlotEnsureExists()
status, err := e.beads.MergeSlotAcquire(holder, false)
```

The formula path doesn't call any merge-slot setup. If the formula created a conflict task and someone manually slung it with `mol-polecat-conflict-resolve`:
1. Polecat would try `bd merge-slot acquire --wait`
2. The slot may not exist yet (never created by `MergeSlotEnsureExists()`)
3. The `bd merge-slot` command may handle this gracefully (auto-create on first acquire) or may error — **this has never been tested**

### 4.7 Summary: six interconnected gaps

```
Gap 1: Format incompatibility
  Formula creates prose → conflict-resolve formula expects structured ## Metadata
  Result: polecat can't extract metadata from task description

Gap 2: No MR blocking
  Formula doesn't call AddDependency → MR stays in queue
  Result: infinite retry loop, duplicate conflict tasks every patrol cycle

Gap 3: No dispatch
  No agent scans bd ready → conflict task sits indefinitely
  Result: manual human intervention required

Gap 4: No notification
  Formula doesn't send MERGE_FAILED for conflicts → witness unaware
  Result: no mail-based trigger for conflict resolution

Gap 5: No merge-slot setup
  Formula doesn't call MergeSlotEnsureExists → slot may not exist
  Result: conflict-resolve polecat may error on slot acquisition

Gap 6: Never exercised
  Zero conflict tasks in beads DB → pipeline entirely theoretical
  Result: all five gaps above are latent, none have been discovered via production use
```

These gaps compound: even if Gap 3 (dispatch) were fixed, Gaps 1 and 5 would prevent the polecat from succeeding. Even if all gaps were fixed in the formula, Gap 2 would still create duplicate tasks until the MR is resolved.

**The Engineer's dead code addresses Gaps 1, 2, 4, and 5.** It creates structured metadata (Gap 1), blocks the MR (Gap 2), integrates with the MERGE_FAILED handler (Gap 4), and sets up the merge slot (Gap 5). Wiring it into a production command would close most of these gaps simultaneously.

---

## 5. Issue Inventory

All issues identified during this investigation, sorted by severity. Severity considers both impact and likelihood — P0 issues would cause immediate failures or data integrity problems when triggered; P3 issues are technical debt that doesn't affect current users.

| # | Severity | Issue | Section | Impact | Affected Code |
|---|----------|-------|---------|--------|---------------|
| 1 | **P0** | **Infinite retry loop / duplicate conflict tasks** — Formula doesn't block MR on conflict task. Each patrol cycle (30s) retries the same branch, creates another duplicate task. After N cycles: N identical tasks, MR still stuck. | 4.3 | Unbounded bead pollution, wasted compute, confusing `bd ready` output | `mol-refinery-patrol.formula.toml` (process-branch step) |
| 2 | **P0** | **Format incompatibility between conflict task creation and consumption** — Refinery formula creates prose metadata (`## Conflict Resolution Required`, no list markers). Conflict-resolve formula expects structured `## Metadata` with `- Field: value` syntax. Polecat can't extract metadata it needs. | 4.2 | Conflict resolution polecat fails at step 1 (load-task), entire resolution fails | `mol-refinery-patrol.formula.toml` ↔ `mol-polecat-conflict-resolve.formula.toml` |
| 3 | **P0** | **`gt mq integration status` reports 0 MRs** — Queries with `Type: "merge-request"` but MR beads have `Type: "task"` with label `gt:merge-request`. Julian confirmed 0 MRs in manual testing. | Plan | Integration status command is non-functional | `mq_integration.go:662, 797` |
| 4 | **P1** | **No agent auto-dispatches conflict tasks** — Conflict task sits in `bd ready` indefinitely. No witness, deacon, boot, or refinery agent scans for unassigned conflict work. Manual `gt sling` with explicit formula required. | 4.4 | Conflict resolution never starts without human intervention — breaks automated pipeline | All patrol formulas (witness, deacon, boot) |
| 5 | **P1** | **MERGE_FAILED protocol silent on conflicts** — Formula sends MERGE_FAILED for test failures only. Conflicts create a task silently — no mail notification to any agent. Even if witness had a conflict-dispatch handler, it would never fire. | 4.5 | No agent is aware a conflict occurred | `mol-refinery-patrol.formula.toml` (handle-failures step) |
| 6 | **P1** | **Merge slot setup only in dead code** — `mol-polecat-conflict-resolve` expects merge slot (`bd merge-slot acquire --wait`). Slot creation (`MergeSlotEnsureExists`) only exists in Engineer's dead `createConflictResolutionTaskForMR()`. Unknown if `bd merge-slot acquire` auto-creates. | 4.6 | Conflict-resolve polecat may error on slot acquisition, or skip serialization | `engineer.go:702-724` (dead), `mol-polecat-conflict-resolve.formula.toml` (step 2) |
| 7 | **P1** | **LLM-dependent merge-push sequence** — Claude must remember polecat name, verify SHA, send MERGED mail, close MR bead, archive message — all in sequence. Any dropped step causes silent lifecycle breakage. | 3 | Orphaned worktrees, orphaned beads, inbox bloat — all silent failures | `mol-refinery-patrol.formula.toml` (merge-push step) |
| 8 | **P1** | **LLM-dependent branch substitution** — Claude must substitute correct branch names into git commands from prose instructions. Wrong branch → merges wrong code or loses work. | 3 | Wrong code merged to main or integration branch | `mol-refinery-patrol.formula.toml` (process-branch step) |
| 9 | **P2** | **`gt sling` auto-applies wrong formula for conflict tasks** — Lines 488-494 auto-apply `mol-polecat-work` for any bare bead slung to polecats. No detection of conflict tasks by title prefix. Matters when/if automated dispatch is added. | 3 | Conflict task treated as generic work — wrong workflow, no merge-slot, wrong merge path | `sling.go:488-494` |
| 10 | **P2** | **LLM-dependent test failure diagnosis** — Claude must determine if test failure is a branch regression vs pre-existing on target. Wrong diagnosis → merges broken code OR rejects good code. | 3 | Broken code lands on main, or valid work blocked indefinitely | `mol-refinery-patrol.formula.toml` (handle-failures step) |
| 11 | **P2** | **LLM-dependent `auto_land` FORBIDDEN enforcement** — Formula relies on Claude respecting FORBIDDEN directive against landing integration branches when `auto_land=false`. No code enforcement except pre-push hook. | 3 | Integration branch landed autonomously when it shouldn't be — bypasses human review gate | `mol-refinery-patrol.formula.toml` (check-integration-branches step) |
| 12 | **P2** | **LLM-dependent inbox parsing** — Claude must parse MERGE_READY mail and remember branch, issue, polecat name, MR bead ID across multiple later steps. Forgetting polecat name → MERGED notification fails. | 3 | Polecat worktrees accumulate indefinitely (no MERGED → witness never nukes) | `mol-refinery-patrol.formula.toml` (inbox-check step) |
| 13 | **P2** | **Conflict resolution pipeline never exercised** — Zero conflict tasks, zero MR beads, empty merge queue in beads DB. Entire conflict pathway is theoretical. All gaps above are latent. | 4.1 | All conflict-related issues undiscoverable until first real conflict | Entire conflict pipeline |
| 14 | **P2** | **Merge strategy divergence** — Formula uses rebase + `merge --ff-only` (linear history). Engineer uses `merge --squash` (single commit). Different commit histories on same inputs. If Engineer is ever wired in, behavior changes. | 3 | Unexpected history changes if switching from formula to Engineer path | `engineer.go:doMerge()` vs formula process-branch step |
| 15 | **P3** | **Engineer merge methods are dead code** — `ProcessMR()`, `ProcessMRInfo()`, `doMerge()`, `handleSuccess()`, `HandleMRInfoSuccess()`, `HandleMRInfoFailure()` have zero callers. ~400 lines of untested, unused code. | 2 | Code maintenance burden; misleading to contributors who think it's used | `engineer.go:264-820` |
| 16 | **P3** | **Test mock `makeTestMR` creates unrealistic beads** — Uses `Type: "merge-request"` instead of `Type: "task"` with `Labels: ["gt:merge-request"]`. Mock `List` filters on `issue.Type` without compat shim. Tests pass against wrong data model. | Plan | Tests give false confidence — pass with mock but fail in production | `mq_testutil_test.go` |
| 17 | **P3** | **Formula FORBIDDEN directives untestable in Go** — Claude-level guardrails can't be validated by automated tests. Only the pre-push hook is a code-enforceable guardrail. | 3 | No automated regression protection for landing restrictions | `mol-refinery-patrol.formula.toml` |

### Summary by severity

| Severity | Count | Theme |
|----------|-------|-------|
| **P0** | 3 | Active bugs: duplicate tasks, format mismatch, broken status command |
| **P1** | 5 | Missing automation: no dispatch, no notification, no slot setup, LLM-dependent critical path |
| **P2** | 5 | Latent risks: wrong formula routing, LLM-dependent decisions, untested pipeline, strategy divergence |
| **P3** | 3 | Technical debt: dead code, unrealistic test data, untestable guardrails |

### Relationship between issues

Issues 1-6 and 13 are all part of the conflict resolution pipeline and compound on each other. Even fixing any single gap, the others prevent end-to-end success:

```
Issue 1 (duplicate tasks) ← needs Issue 4 (dispatch) to be fixed, but
Issue 4 (dispatch)        ← needs Issue 2 (format) to be fixed, but
Issue 2 (format)          ← needs Issue 6 (merge slot) to be fixed
Issue 5 (no notification) ← independent, but same pipeline
Issue 13 (never tested)   ← all of the above are latent because of this
```

The Engineer's dead code (Issue 15) addresses Issues 1, 2, 5, and 6 simultaneously. Wiring it into a production command (`gt refinery process-next`) is the highest-leverage fix — it closes 4 of 6 conflict pipeline gaps and converts LLM-dependent merge mechanics (Issues 7, 8) into deterministic code.

Issue 3 (`gt mq integration status` reports 0 MRs) is independent and has a straightforward fix (query by Label instead of Type).

---

## 6. Existing Test Infrastructure

### Test helpers already available

| Helper | Location | Purpose |
|--------|----------|---------|
| `buildGT(t)` | `internal/cmd/test_helpers_test.go` | Compiles `gt` binary, caches across tests |
| `createTestGitRepo(t, name)` | `internal/cmd/rig_integration_test.go` | Creates git repo with initial commit on `main` |
| `setupTestTown(t)` | `internal/cmd/rig_integration_test.go` | Creates `townRoot/mayor/rigs.json` + `.beads/` |
| `mockBdCommand(t)` | `internal/cmd/rig_integration_test.go` | Fake `bd` binary on PATH (handles init, create, show) |
| `cleanGTEnv(t)` | various | Strips `GT_*` env vars |

### CI infrastructure

- **`ci.yml`** integration job: builds `bd` from source, builds `gt`, runs `go test -tags=integration -timeout=5m`
- **`integration.yml`**: path-filtered trigger, 8-min timeout, same setup
- Both set `git config --global user.name "CI Bot"`
- No dolt server in CI. No daemon. No tmux.
- 10 existing integration test files with `//go:build integration` tag
- No dedicated e2e test category exists today

### Existing refinery tests

`internal/refinery/engineer_test.go` tests config loading and `NewEngineer` construction with `rig.Rig{Name: "test-rig", Path: tmpDir}`. These are unit tests — no merge operations are tested.

---

## 7. Proposed Architecture

### New build tag: `e2e`

```go
//go:build e2e
```

**Why not `integration`?**
- `integration` tag is already used for 10 test files with 5-8 min timeouts
- E2E tests will be heavier (Part C lifecycle could take 30-60s per subtest)
- Separate tag allows separate CI trigger rules and timeout budgets

### Package structure

```
internal/e2e/
  testutil/
    town.go           ← SetupTestTown, SetupTestRig, SetupTestGitRemote
    beads.go           ← SetupMockBeads (or real bd with tmpdir)
    engineer.go        ← SetupTestEngineer (wraps refinery.NewEngineer)
    git.go             ← Helpers for creating branches, commits, worktrees
    assertions.go      ← AssertBranchExists, AssertBranchDeleted, AssertCommitOn
  integration_branch_test.go  ← All 28 tests (subtests within TestIntegrationBranch*)
  (future: sling_test.go, convoy_test.go, etc.)
```

**Why a new `e2e` package (not inside `internal/cmd`)?**
- `internal/cmd` tests are tightly coupled to cobra commands and `cmd` package internals
- E2E tests need to import from multiple packages (`refinery`, `beads`, `git`, `rig`, `config`)
- Separate package forces clean API boundaries
- Future e2e tests for other features (sling, convoy, etc.) can share the same `testutil` fixtures

### CI workflow

```yaml
# .github/workflows/e2e.yml
name: E2E Tests
on:
  pull_request:
    paths:
      - 'internal/refinery/**'
      - 'internal/cmd/mq_*'
      - 'internal/cmd/done*'
      - 'internal/cmd/sling*'
      - 'internal/e2e/**'
      - '.github/workflows/e2e.yml'
jobs:
  e2e:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      # ... standard Go setup, build bd, build gt ...
      - run: go test -v -tags=e2e -timeout=10m ./internal/e2e/...
```

---

## 8. Challenges & Mitigations

### Challenge 1: Beads (`bd`) dependency

The beads CLI talks to a Dolt SQL server in production. CI doesn't have dolt running.

| Approach | Pros | Cons |
|----------|------|------|
| **A: Mock bd** (like existing tests) | Fast, no dolt needed | Doesn't test real bead CRUD; MR fields may drift |
| **B: Real bd with embedded mode** | Tests real paths | Embedded dolt crashes in certain CWDs; slow init |
| **C: Real bd with flat-file backend** | Tests real paths, no dolt | Needs verification that `bd` supports this |
| **D: Build bd from source + dolt server** | Full fidelity | Complex CI setup, slow |

**Recommended:** Start with **A (mock bd)**, graduate to C if flat-file works. The existing `mockBdCommand` pattern is proven. The things we care about testing (git operations, branch targeting, merge mechanics) don't depend on beads persistence fidelity.

### Challenge 2: Git remote operations

The Engineer calls `git push origin` and `git pull origin`. In tests, "origin" needs to be a local bare repo, not GitHub.

**Mitigation:** Already solved by `createTestGitRepo(t, name)`. We create a bare repo on the filesystem and use it as the remote:

```
tmpDir/
  remote.git/           ← bare repo (simulates GitHub)
  town/
    testrig/
      refinery/rig/     ← clone of remote.git (refinery's worktree)
      mayor/rig/        ← clone of remote.git (mayor's clone)
      polecats/nux/     ← git worktree (simulates polecat)
      config.json
```

### Challenge 3: Polecat simulation

Real polecats are git worktrees created by the Witness/Sling system, backed by Claude sessions. We don't need any of that for testing.

A polecat's contribution to the lifecycle is just:
1. Create a git worktree from the rig's shared `.repo.git`
2. Make commits on a branch named `polecat/<name>`
3. Push the branch
4. Call `gt done` (which creates an MR bead)

For testing, we can simulate steps 1-3 with raw git commands and step 4 by directly creating an MR bead via the mock `bd`.

### Challenge 4: `gt done` is a cobra command

`gt done` discovers the workspace via CWD, reads rig config, finds the current polecat's branch, and creates an MR bead. It's heavily tied to the `cmd` package.

| Approach | Feasibility |
|----------|-------------|
| Call `gt done` as subprocess | Works — we have the built binary from `buildGT(t)` |
| Extract MR creation into a testable function | Cleaner but large refactor |

**Recommended:** Subprocess approach. Run `gt done` from within the simulated polecat worktree. The mock `bd` handles bead creation. This tests the actual binary path.

### Challenge 5: `gt mq integration land` has guardrails

The land command pushes to main, which triggers the pre-push hook. The hook checks for `GT_INTEGRATION_LAND=1`.

**Mitigation:** In tests, we can:
- Test **with** hook installed (verify guardrails work — blocked without env var)
- Test **with** `GT_INTEGRATION_LAND=1` env (verify the bypass works)
- Test **without** hook (baseline merge works)

### Challenge 6: `auto_land` config (from PR #1226)

The `integration_branch_auto_land` config controls whether the refinery can autonomously land integration branches. Default is `false`. The formula FORBIDDEN directives and pre-push hook enforce this.

Tests needed:
- `auto_land=false` (default) → `gt mq integration status` reports auto-land disabled
- `auto_land=true` → `gt mq integration status` reports auto-land enabled, `ready_to_land` reflects epic state
- Interplay between `auto_land` config and pre-push hook `GT_INTEGRATION_LAND` env var
- Note: The formula's FORBIDDEN directives are Claude-level guardrails — they can't be tested in Go code. The pre-push hook is the enforceable guardrail we can test.

### Challenge 7: Local vs CI runability

**Both should work.** The test scaffolding uses `t.TempDir()`, mock binaries, and local git repos. No external services needed. CI-specific concerns:
- Build tags (`-tags=e2e`) need to be in a workflow
- Timeout budget: Part C lifecycle tests need ~2 min total
- `bd` binary needs to be on PATH (mock or real)

---

## 9. Test Coverage Mapping

### Julian's 28 tests → Automated tests

| Part | Tests | Automation Approach |
|------|-------|-------------------|
| **A: Unit** (A1-A3) | Config parsing, branch naming, target detection | Already exist as unit tests; add any missing |
| **B: Regression** (B1-B3) | Pre-closed epic, stale ref, duplicate create | Pure Go tests calling `runMqIntegration*` functions |
| **C: Lifecycle** (C1-C8) | Create → Sling → MR → Merge → Land | Full scaffolding: bare repo + town + engineer |
| **D: Config** (D1-D4) | Template overrides, `auto_land=false` | Config file variations, subprocess calls |
| **E: Edge** (E1-E5) | Conflicts, concurrent MRs, force push | Git state manipulation |
| **F: Deprecation** (F1-F3) | Doctor checks, settings migration | `gt doctor` subprocess calls |

### Part C test flow (the crown jewel)

```
TestIntegrationBranchLifecycle(t *testing.T):
  1. SetupTestTown(t)                  → town with rig, bare git remote
  2. gt mq integration create <epic>   → creates integration branch (OUR CODE)
  3. Simulate polecat work             → git worktree, commit, push branch
  4. Create MR bead (target: integration/...)
  5. gt refinery ready                 → verify MR appears in queue (OUR CODE)
  6. Simulate merge (raw git commands) → rebase + merge + push (MIMICS CLAUDE)
  7. Verify: integration branch has the commit
  8. gt mq integration land            → merges integration → main (OUR CODE)
  9. Verify: main has the commit, integration branch deleted
```

Note: Step 6 uses raw git commands (what Claude would do following the formula), not
`Engineer.ProcessMRInfo()` which has zero callers in production. Steps 2, 5, and 8
test our actual code paths.

---

## 10. Open Questions

1. **Mock bd vs real bd?** Mock is simpler and faster. Real bd tests more but needs either flat-file backend or dolt. Recommend starting with mock.

2. **How to handle the merge step in Part C?** The production merge is Claude running git commands — our code doesn't participate. Recommend simulating the merge with raw git commands in the test (rebase + merge + push), same as what Claude does. This tests the full lifecycle end-to-end while focusing our assertions on the code boundaries we own (create, target detection, queue listing, land).

3. **New `e2e.yml` workflow vs existing `ci.yml`?** New workflow with path filters — e2e tests only run when integration-related code changes.

4. **Should `testutil` fixtures live in the e2e package or be shared?** Start in `internal/e2e/testutil/`. If other packages need them later, promote to `internal/testutil/`.

5. **What to do about Engineer's unused merge methods?** `ProcessMR()`, `ProcessMRInfo()`, `doMerge()`, `handleSuccess()`, `HandleMRInfoSuccess()`, `HandleMRInfoFailure()` have zero callers in the entire codebase. Options: (a) leave as-is (harmless dead code), (b) wire them into a `gt refinery process` command so the production path has a programmatic option, (c) remove them. If (b), then e2e tests could call the command instead of simulating git operations. **Section 4 strengthens the case for (b)**: the Engineer's dead code already solves five of the six conflict resolution gaps (format compatibility, MR blocking, notification, merge-slot setup, and serialization). Wiring it in would simultaneously fix the conflict pipeline and make it testable. This is a design question for the upstream maintainers.

6. **Should the conflict resolution pipeline be fixed before e2e tests are written?** Section 4 documents six compounding gaps. Writing e2e tests for the conflict path (Julian's Part E: E1-E5) would essentially be testing a pipeline that doesn't work end-to-end. Options: (a) write tests that document the current broken behavior (test-as-specification), (b) fix the pipeline first then write tests against the fixed behavior, (c) write tests against the *intended* behavior (Engineer's approach) as a forcing function for wiring it in.

7. **Is the `bd merge-slot` command resilient to missing slot?** The conflict-resolve formula tells the polecat to run `bd merge-slot acquire --wait`, but the slot creation (`MergeSlotEnsureExists`) only exists in the Engineer's dead code. Does `bd merge-slot acquire` auto-create the slot, or does it error? This needs testing.

---

## 11. References

- Julian's manual test plan: https://gist.github.com/julianknutsen/04217bd547d382ce5c6e37f44d3700bf
- Xexr's adapted test plan: https://gist.github.com/Xexr/f6c6902fe818aad7ccdf645d21e08a49
- Engineer source: `internal/refinery/engineer.go`
- Existing integration tests: `internal/cmd/rig_integration_test.go`
- CI workflow: `.github/workflows/ci.yml`
- Pre-push hook: `.githooks/pre-push`
