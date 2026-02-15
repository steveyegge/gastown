# Implementation Plan: `gt refinery process-next`

> **The biggest win**: Wire the Engineer's existing dead merge code into a single CLI command.
> This one change fixes 4 of 6 conflict resolution pipeline gaps, makes the merge path
> deterministic (model-independent), and enables automated e2e testing.

## Why This Is the Biggest Win

The investigation ([issue-tracking.md](./e2e/issue-tracking.md)) found 10 pre-existing issues
in the merge queue and conflict resolution pipeline. They cluster into two groups:

1. **Conflict resolution pipeline** (Issues 1, 2, 4, 5, 6, 13) — six compounding gaps
2. **LLM-dependent formula steps** (Issues 7, 8, 10, 12) — model-dependent behavior

Both groups share a root cause: the merge mechanics are prose instructions that Claude follows,
not deterministic code. The Engineer struct already has the code — `ProcessMRInfo()`, `doMerge()`,
`HandleMRInfoSuccess()`, `HandleMRInfoFailure()` — but it's dead (zero callers).

Wiring it into `gt refinery process-next` simultaneously:

| What it fixes | How |
|---------------|-----|
| Issue 1: Infinite retry / duplicate tasks | `HandleMRInfoFailure` calls `AddDependency(mr, task)` — blocks MR |
| Issue 2: Format incompatibility | `createConflictResolutionTaskForMR` writes structured `## Metadata` that the conflict-resolve formula expects |
| Issue 5: MERGE_FAILED silent on conflicts | `HandleMRInfoFailure` sends `MERGE_FAILED` mail for conflicts AND test failures |
| Issue 6: Merge slot not set up | `createConflictResolutionTaskForMR` calls `MergeSlotEnsureExists` + `MergeSlotAcquire` |
| Issues 7, 8: LLM-dependent merge sequence | Go code carries `MRInfo` through the pipeline — no values to forget |
| Issue 14: Merge strategy divergence | Configurable `MergeStrategy` — rebase-ff (formula) or squash (original Engineer) |
| Testability | Deterministic code callable from Go tests — no Claude simulation needed |

**What it doesn't fix** (separate work):
- Issue 4: No auto-dispatch (needs witness/dispatcher changes)
- Issue 9: Sling formula routing (needs sling.go changes)
- Issue 10: LLM-dependent test failure diagnosis (general formula concern)
- Issue 13: Never exercised (fixed by writing e2e tests — Phase 2 below)

---

## Architecture

### Current: Formula drives merge

```
Formula (prose) → Claude (LLM) → git commands (shell)
                                → bd commands (shell)
                                → gt mail send (shell)
```

Claude must remember values across steps, construct commands correctly, execute
them in order, and handle all error paths. Model-dependent.

### Proposed: Command drives merge, formula orchestrates

```
Formula (prose) → Claude (LLM) → gt refinery process-next (Go)
                                      ├─ git operations (deterministic)
                                      ├─ bead updates (deterministic)
                                      ├─ mail notifications (deterministic)
                                      └─ conflict task creation (deterministic)
```

Claude's role shrinks to: "run `gt refinery process-next`, read output, decide
whether to continue patrol or hand off." The merge mechanics — the critical path
where bugs live — become deterministic code.

### Command interface

```bash
# Process the next ready MR in the queue
gt refinery process-next [rig-name]

# The target branch comes from the MR bead's Target field (mr.Target).
# This is set at MR creation time — the command never assumes "main".
# Target will be either:
#   - The rig's default branch (e.g., "main") for normal MRs
#   - An integration branch (e.g., "integration/l0g1x") for integration MRs

# Output (merging to default branch):
#   Processing MR gt-abc: polecat/Nux/gt-001 → main
#   Rebasing onto origin/main...
#   Running tests: go test ./...
#   Merging (ff-only) into main...
#   Pushing to origin/main...
#   Sending MERGED notification to witness...
#   Closing MR bead gt-abc...
#   Closing source issue gt-def...
#   Deleting branch polecat/Nux/gt-001...
#   Done: merged at abc1234

# Output (merging to integration branch):
#   Processing MR gt-abc: polecat/Nux/gt-001 → integration/l0g1x
#   Rebasing onto origin/integration/l0g1x...
#   Running tests: go test ./...
#   Merging (ff-only) into integration/l0g1x...
#   Pushing to origin/integration/l0g1x...
#   Sending MERGED notification to witness...
#   Closing MR bead gt-abc...
#   Source issue gt-def left open (integration branch)...
#   Deleting branch polecat/Nux/gt-001...
#   Done: merged at abc1234

# On conflict:
#   Processing MR gt-abc: polecat/Nux/gt-001 → integration/l0g1x
#   Rebasing onto origin/integration/l0g1x...
#   CONFLICT: 3 files have conflicts
#   Creating conflict resolution task...
#   Blocking MR gt-abc on task gt-xyz...
#   Acquiring merge slot...
#   Sending MERGE_FAILED to witness...
#   Done: conflict — task gt-xyz created for resolution

# On test failure:
#   Processing MR gt-abc: polecat/Nux/gt-001 → main
#   Rebasing onto origin/main...
#   Running tests: go test ./...
#   FAILED: 2 test failures
#   Sending MERGE_FAILED to witness...
#   Reopening source issue gt-def...
#   Closing MR bead gt-abc (rejected)...
#   Done: rejected — test failures
```

Exit codes are communicated via the command's return error, not `os.Exit()` (see
Phase 3 implementation for details). The cobra `RunE` handler returns a typed
`ProcessExitError` so deferred cleanup (like `ReleaseMR`) runs before exit:
- 0: merged successfully
- 1: conflict (task created)
- 2: test failure (MR rejected)
- 3: queue empty (nothing to process)
- 4: error (infrastructure failure)

---

## Implementation Phases

> **Test-first ordering**: Tests are written BEFORE the code they validate.
> Each phase includes its own tests. Failing tests (RED) are committed first,
> then implementation makes them pass (GREEN).

### Phase 1: Add missing git operations

**File: `internal/git/git.go`**

Add `MergeFFOnly` method (doesn't exist yet — confirmed absent from codebase):

```go
// MergeFFOnly performs a fast-forward-only merge. Returns error if not possible.
func (g *Git) MergeFFOnly(branch string) error {
    _, err := g.run("merge", "--ff-only", branch)
    return err
}
```

Add configurable merge strategy to `MergeQueueConfig`:

```go
// MergeStrategy controls how branches are merged: "rebase-ff" (default) or "squash".
// "rebase-ff" matches the production formula: rebase onto target, then ff-only merge.
// "squash" is the original Engineer behavior: squash merge into single commit.
MergeStrategy string `json:"merge_strategy"`
```

Default: `"rebase-ff"` — matches what the production formula does.

**Modify `doMerge()`** to support both strategies:

The current `doMerge` (engineer.go:264-382) flow is:
`BranchExists → Checkout target → Pull → CheckConflicts → RunTests → MergeSquash → Push`

The rebase-ff strategy differs in two ways:
1. **Conflict detection**: Instead of a dry-run `CheckConflicts`, the rebase itself
   detects conflicts (attempt-and-abort). This is correct — rebase failure IS conflict.
2. **Merge method**: `Rebase + MergeFFOnly` instead of `MergeSquash`.
3. **Test ordering**: Tests run AFTER rebase on the rebased branch, not on the target.

Extract shared steps and branch on strategy:

```go
func (e *Engineer) doMerge(ctx context.Context, branch, target, sourceIssue string) ProcessResult {
    // Shared: verify branch exists
    exists, err := e.git.BranchExists(branch)
    if !exists { return ProcessResult{...} }

    if e.config.MergeStrategy == "squash" {
        // --- Existing squash path (unchanged) ---
        e.git.Checkout(target)
        e.git.Pull("origin", target)
        conflicts, _ := e.git.CheckConflicts(branch, target)
        if len(conflicts) > 0 { return ProcessResult{Conflict: true, ...} }
        // Tests run on target with squashed changes
        if e.config.RunTests { ... }
        e.git.MergeSquash(branch, msg)
        e.git.Push("origin", target, false)
    } else {
        // --- Rebase + ff-only (matches production formula) ---
        // 1. Fetch latest target
        e.git.Fetch("origin", target)
        // 2. Rebase branch onto target (conflict detection happens here)
        e.git.Checkout(branch)
        if err := e.git.Rebase("origin/" + target); err != nil {
            e.git.AbortRebase()
            return ProcessResult{Conflict: true, Error: "rebase conflict: " + err.Error()}
        }
        // 3. Tests run on the REBASED branch (important: after rebase, before merge)
        if e.config.RunTests && e.config.TestCommand != "" {
            result := e.runTests(ctx)
            if !result.Success {
                return ProcessResult{TestsFailed: true, Error: result.Error}
            }
        }
        // 4. Fast-forward target to rebased branch
        e.git.Checkout(target)
        e.git.MergeFFOnly(branch)
        // 5. Push
        e.git.Push("origin", target, false)
    }

    mergeCommit, _ := e.git.Rev("HEAD")
    return ProcessResult{Success: true, MergeCommit: mergeCommit}
}
```

**Estimated scope**: ~10 lines in git.go, ~50 lines in engineer.go

### Phase 2: Write tests (RED)

> Tests are written FIRST. They will fail until Phases 3-4 implement the code.

**File: `internal/refinery/engineer_test.go`** (new or appended)

Unit tests for the Engineer methods. These test the Go functions directly —
no subprocess calls, no git repos needed for pure logic tests.

```go
// Test: doMerge with rebase-ff strategy produces correct git sequence
func TestDoMerge_RebaseFF_HappyPath(t *testing.T) {
    // Setup: mock git, mock beads, engineer with MergeStrategy: "rebase-ff"
    // Call: eng.doMerge(ctx, "feature", "main", "issue-1")
    // Assert: git operations in order: Fetch, Checkout(branch), Rebase, Checkout(target), MergeFFOnly, Push
    // Assert: result.Success == true, MergeCommit != ""
}

func TestDoMerge_RebaseFF_Conflict(t *testing.T) {
    // Setup: mock git with Rebase returning error
    // Call: eng.doMerge(ctx, "feature", "main", "issue-1")
    // Assert: AbortRebase called
    // Assert: result.Conflict == true, result.Success == false
}

func TestDoMerge_RebaseFF_TestFailure(t *testing.T) {
    // Setup: mock git (rebase succeeds), mock test command that fails
    // Call: eng.doMerge(ctx, "feature", "main", "issue-1")
    // Assert: tests run AFTER rebase (on rebased branch, not target)
    // Assert: result.TestsFailed == true, result.Success == false
}

func TestDoMerge_Squash_HappyPath(t *testing.T) {
    // Setup: mock git, engineer with MergeStrategy: "squash"
    // Call: eng.doMerge(ctx, "feature", "main", "issue-1")
    // Assert: CheckConflicts, MergeSquash called (NOT Rebase, NOT MergeFFOnly)
    // Assert: result.Success == true
}

func TestHandleMRInfoSuccess_DeletesBranch(t *testing.T) {
    // Setup: engineer with DeleteMergedBranches: true
    // Call: eng.HandleMRInfoSuccess(mr, successResult)
    // Assert: DeleteBranch(branch, true) called
    // Assert: DeleteRemoteBranch("origin", branch) called
}

func TestHandleMRInfoFailure_DoesNotDeleteBranch(t *testing.T) {
    // Setup: same config (DeleteMergedBranches: true)
    // Call with conflict result: eng.HandleMRInfoFailure(mr, conflictResult)
    // Assert: DeleteBranch NOT called
    // Assert: DeleteRemoteBranch NOT called
    // (branch must survive for conflict resolution)
}

func TestHandleMRInfoFailure_TestFailure_DoesNotDeleteBranch(t *testing.T) {
    // Setup: same config
    // Call with test failure: eng.HandleMRInfoFailure(mr, testFailResult)
    // Assert: DeleteBranch NOT called
    // Assert: DeleteRemoteBranch NOT called
    // (branch must survive for rework)
}
```

**File: `internal/refinery/engineer_integration_test.go`** (new, `//go:build integration`)

Integration tests with real git repos. These validate the full pipeline.

```go
func TestProcessNextMR_HappyPath(t *testing.T) {
    // Setup: bare repo, rig, engineer
    // Create polecat branch with commits, push
    // Create MR bead targeting main
    // Call: eng.ProcessMRInfo(ctx, mr) + eng.HandleMRInfoSuccess(mr, result)
    // Assert: commits on main, MR bead closed, branch deleted
}

func TestProcessNextMR_IntegrationBranch(t *testing.T) {
    // Setup: bare repo with integration/foo branch
    // Create polecat branch, MR bead targeting integration/foo
    // Process
    // Assert: commits on integration/foo (not main), MR closed, source issue OPEN
}

func TestProcessNextMR_Conflict(t *testing.T) {
    // Setup: bare repo, create conflicting changes on main and polecat branch
    // Create MR bead
    // Process
    // Assert: result.Conflict == true
    // Call: eng.HandleMRInfoFailure(mr, result)
    // Assert: conflict task created with structured ## Metadata
    // Assert: MR blocked on task (dependency)
    // Assert: merge slot acquired
    // Assert: MERGE_FAILED mail sent
}

func TestProcessNextMR_TestFailure(t *testing.T) {
    // Setup: configure TestCommand that will fail
    // Create MR bead
    // Process
    // Assert: result.TestsFailed == true
    // Assert: MR rejected, source issue reopened
}

func TestHandleMRInfoSuccess_IntegrationBranch_LeavesSourceOpen(t *testing.T) {
    // Setup: MR with Target: "integration/foo", rig DefaultBranch: "main"
    // Call: eng.HandleMRInfoSuccess(mr, result)
    // Assert: MR bead closed
    // Assert: source issue NOT closed (still open)
}

func TestHandleMRInfoSuccess_DefaultBranch_ClosesSource(t *testing.T) {
    // Setup: MR with Target: "main", rig DefaultBranch: "main"
    // Call: eng.HandleMRInfoSuccess(mr, result)
    // Assert: MR bead closed
    // Assert: source issue closed
}
```

**File: `internal/e2e/integration_branch_test.go`** (new, `//go:build e2e`)

Full lifecycle using subprocess calls:

```go
func TestIntegrationBranchLifecycle(t *testing.T) {
    // 1. gt mq integration create <epic>
    // 2. Simulate polecat work (git branch, commit, push)
    // 3. Create MR bead (target: integration/<name>)
    // 4. gt refinery process-next  ← DETERMINISTIC, not simulated git
    // 5. Assert: integration branch has the commit
    // 6. gt mq integration land <epic>
    // 7. Assert: main has the commit, integration branch deleted
}

func TestIntegrationBranchConflict(t *testing.T) {
    // 1. Create integration branch
    // 2. Create two polecat branches that conflict
    // 3. Process first MR → success
    // 4. Process second MR → conflict
    // 5. Assert: conflict task created, MR blocked
    // 6. Simulate resolution (rebase, force push)
    // 7. Close conflict task
    // 8. Process second MR again → success
}
```

**Estimated scope**: ~400 lines across three test files

At this point: tests compile but FAIL (RED). Phases 3-4 make them pass.

### Phase 3: Create `gt refinery process-next` command (GREEN)

**File: `internal/cmd/refinery.go`**

Add new cobra command following the existing pattern (see `runRefineryClaim` at line 567):

```go
var refineryProcessNextCmd = &cobra.Command{
    Use:   "process-next [rig]",
    Short: "Process the next ready MR in the merge queue",
    Long: `Deterministically processes the next unclaimed, unblocked MR.

Performs: rebase → test → merge → push → notify → close bead.
On conflict: creates resolution task, blocks MR, notifies witness.
On test failure: rejects MR, notifies witness, reopens source issue.`,
    Args: cobra.MaximumNArgs(1),
    RunE: runRefineryProcessNext,
}
```

**Implementation of `runRefineryProcessNext`:**

```go
// ProcessExitError wraps a process result with an exit code.
// This allows deferred cleanup (ReleaseMR) to run before the process exits.
type ProcessExitError struct {
    ExitCode int
    Message  string
}

func (e *ProcessExitError) Error() string { return e.Message }

func runRefineryProcessNext(cmd *cobra.Command, args []string) error {
    // 1. Get rig (same pattern as runRefineryClaim)
    townRoot, err := workspace.FindFromCwdOrError()
    if err != nil { return err }
    rigName, err := inferRigFromCwd(townRoot)
    if err != nil && len(args) > 0 { rigName = args[0] }
    _, r, err := getRig(rigName)
    if err != nil { return err }

    eng := refinery.NewEngineer(r)
    eng.LoadConfig()

    // 2. Find next MR
    ready, err := eng.ListReadyMRs()
    if err != nil { return err }
    if len(ready) == 0 {
        fmt.Println("Queue empty")
        return &ProcessExitError{ExitCode: 3, Message: "queue empty"}
    }
    mr := ready[0]

    // 3. Claim it (ReleaseMR takes 1 arg — mrID only)
    workerID := getWorkerID()
    if err := eng.ClaimMR(mr.ID, workerID); err != nil {
        return fmt.Errorf("claiming MR: %w", err)
    }
    defer eng.ReleaseMR(mr.ID)  // Release on any path — runs before exit

    // 4. Process it
    ctx := cmd.Context()
    result := eng.ProcessMRInfo(ctx, mr)

    // 5. Handle result — return typed error for exit code
    if result.Success {
        eng.HandleMRInfoSuccess(mr, result)
        return nil  // exit 0
    } else if result.Conflict {
        eng.HandleMRInfoFailure(mr, result)
        return &ProcessExitError{ExitCode: 1, Message: "conflict — task created"}
    } else {
        eng.HandleMRInfoFailure(mr, result)
        return &ProcessExitError{ExitCode: 2, Message: "test failure — MR rejected"}
    }
}
```

**Key details verified against actual code:**
- `NewEngineer(r)` initializes `e.router` via `mail.NewRouter(r.Path)` — mail works in CLI context
- `ReleaseMR(mrID string)` takes 1 argument (not 2) — clears assignee
- `ClaimMR(mrID, workerID string)` takes 2 arguments — sets assignee
- `ProcessMRInfo(ctx, *MRInfo)` returns `ProcessResult` with `Success`, `Conflict`, `TestsFailed` flags
- `ListReadyMRs()` uses `ReadyWithType("merge-request")` which correctly calls `bd ready --label gt:merge-request` — no Type vs Label bug
- Exit codes use `ProcessExitError` type so `defer ReleaseMR` runs before exit

**Wire into cobra**: Add to `init()` alongside existing refinery subcommands.

**Estimated scope**: ~80 lines in refinery.go

### Phase 4: Fix `HandleMRInfoSuccess` for integration branches (GREEN)

The current `HandleMRInfoSuccess` (engineer.go:624-638) closes source issue
unconditionally. For integration branch merges, the source issue should stay
open (it gets closed when the integration branch lands to main).

**Verified**: `rig.DefaultBranch()` exists at `rig/types.go:88`. Returns
configured default branch, falls back to `"main"`.

```go
// In HandleMRInfoSuccess, replace unconditional source issue close (lines 624-638):
if mr.SourceIssue != "" {
    defaultBranch := e.rig.DefaultBranch()
    if mr.Target == defaultBranch {
        // Merging to default branch — close the source issue
        closeReason := fmt.Sprintf("Merged in %s", mr.ID)
        e.beads.CloseWithReason(closeReason, mr.SourceIssue)
        convoy.CheckConvoysForIssue(e.rig.Path, mr.SourceIssue, "refinery", logger)
    } else {
        // Merging to integration branch — leave source issue open
        fmt.Fprintf(e.output, "[Engineer] Source issue %s left open (merged to integration branch %s)\n",
            mr.SourceIssue, mr.Target)
    }
}
```

**Estimated scope**: ~15 lines in engineer.go

At this point: all unit and integration tests from Phase 2 should PASS (GREEN).

### Phase 5: Update formula to use the new command

**File: `internal/formula/formulas/mol-refinery-patrol.formula.toml`**

Replace the process-branch → run-tests → handle-failures → merge-push sequence with:

```toml
[[steps]]
id = "process-next"
title = "Process next merge request"
needs = ["queue-scan"]
description = """
Run the deterministic merge processor:

```bash
gt refinery process-next {{rig_name}}
```

Read the output and exit code:
- Exit 0: Merged successfully. Note the MR ID and commit SHA from output.
- Exit 1: Conflict detected. A conflict resolution task was created automatically.
  Note the task ID from output. Skip to loop-check.
- Exit 2: Test failure. The MR was rejected automatically.
  Note the failure reason from output. Skip to loop-check.
- Exit 3: Queue empty. Skip to check-integration-branches.
- Exit 4: Infrastructure error. Log the error, skip to loop-check.

The command handles all merge mechanics, notifications, and bead updates
deterministically. You do NOT need to run git commands, send mail, or
close beads yourself.

After exit 0: proceed to loop-check (target branch has moved, remaining
branches need rebasing on new baseline)."""
```

This replaces **four formula steps** (process-branch, run-tests, handle-failures,
merge-push) with **one command invocation**. Claude's job becomes reading output
and deciding what to do next — not executing the merge.

**The formula's inbox-check, queue-scan, loop-check, check-integration-branches,
and context-check steps remain unchanged.** Those are patrol orchestration concerns
that are appropriate for LLM control.

**Estimated scope**: ~30 lines replacing ~200 lines of formula prose

---

## Execution Order

```
Phase 1: git.MergeFFOnly + configurable merge strategy       (~60 lines)
    ↓
Phase 2: Write failing tests (RED)                            (~400 lines)
    ↓
Phase 3: gt refinery process-next command (tests go GREEN)    (~80 lines)
    ↓
Phase 4: Integration branch handling in HandleMRInfoSuccess   (~15 lines)
    ↓    (remaining tests go GREEN)
    ↓
Phase 5: Update formula to use new command                    (~30 lines, -200 lines)
```

Total new code: ~585 lines
Total removed formula prose: ~200 lines
Net: ~385 lines added

Each phase is independently committable. Phase 1 is preparation (new git method).
Phase 2 establishes the contract (what the code must do). Phases 3-4 fulfill
the contract (tests go green). Phase 5 is the formula integration (optional, can defer).

---

## Verified Assumptions

These were verified against the actual codebase during plan review:

| Assumption | Status | Evidence |
|------------|--------|----------|
| `NewEngineer` initializes `e.router` | **Confirmed** | `engineer.go:143` — `mail.NewRouter(r.Path)` |
| `ProcessMRInfo` returns `ProcessResult` | **Confirmed** | `engineer.go:568` — struct has Success, Conflict, TestsFailed, MergeCommit, Error |
| `HandleMRInfoSuccess` closes source issue unconditionally | **Confirmed** | `engineer.go:624-638` — no branch check |
| `HandleMRInfoFailure` sends MERGE_FAILED mail | **Confirmed** | `engineer.go:673` — `e.router.Send(msg)` |
| `HandleMRInfoFailure` creates conflict task + blocks MR | **Confirmed** | `engineer.go:679-693` — `AddDependency(mr.ID, taskID)` |
| `createConflictResolutionTaskForMR` writes `## Metadata` | **Confirmed** | `engineer.go:771-798` — structured format |
| `ListReadyMRs` uses label-based filtering | **Confirmed** | `beads.go:417` — `bd ready --label gt:merge-request` (NOT a Type vs Label bug) |
| `ClaimMR(mrID, workerID)` exists | **Confirmed** | `engineer.go:1089` — 2 args |
| `ReleaseMR(mrID)` exists | **Confirmed** | `engineer.go:1097` — 1 arg (NOT 2) |
| `LoadConfig()` exists | **Confirmed** | `engineer.go:154` — parses config.json merge_queue section |
| `rig.DefaultBranch()` exists | **Confirmed** | `rig/types.go:88` — falls back to "main" |
| `git.Rebase(onto)` exists | **Confirmed** | `git.go:608` |
| `git.AbortRebase()` exists | **Confirmed** | `git.go:662` |
| `git.MergeFFOnly` does NOT exist | **Confirmed** | Absent — must be added (Phase 1) |
| `git.DeleteRemoteBranch(remote, branch)` exists | **Confirmed** | `git.go:563` |
| Cobra command pattern established | **Confirmed** | `refinery.go:567-593` — `getRig → NewEngineer → method call` |

---

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Merge strategy mismatch (squash vs rebase-ff) changes commit history | Default to rebase-ff (matching formula). Squash is opt-in config. Both strategies have dedicated tests. |
| Engineer's `doMerge` rebase-ff path is new code alongside existing squash path | Extract shared steps (verify, push). Unit tests cover both paths independently. |
| `HandleMRInfoSuccess` closes source issue unconditionally | Phase 4 adds integration branch check — only close when merging to default branch. |
| Existing formula-based refineries need migration | Phase 5 is optional — old formula still works. New command is additive. |
| Test ordering in rebase-ff: tests must run on REBASED branch | Explicitly coded: checkout branch → rebase → run tests → checkout target → ff-merge. |

---

## What Changes for Users

**For formula-driven refineries** (current):
- Nothing breaks. The formula continues to work as-is.
- When Phase 5 is deployed, the formula calls `gt refinery process-next` instead of raw git.
- Claude's role shifts from "execute merge" to "decide when to merge."

**For e2e tests** (new):
- Full lifecycle testable without Claude, tmux, or daemon.
- `gt refinery process-next` is a subprocess call, same as `gt mq integration create/land`.

**For operators**:
- `gt refinery process-next` can be run manually to process a stuck MR.
- Conflict resolution tasks have structured metadata and block the MR.
- MERGE_FAILED notifications work for conflicts, not just test failures.

---

## Issue Coverage

Cross-referenced against [issue-tracking.md](./e2e/issue-tracking.md):

| Issue # | Severity | Addressed? | How |
|---------|----------|------------|-----|
| 1 | P0 | **Yes** | `HandleMRInfoFailure` blocks MR with dependency |
| 2 | P0 | **Yes** | `createConflictResolutionTaskForMR` writes structured `## Metadata` |
| 4 | P1 | No (deferred) | Needs witness/dispatcher changes |
| 5 | P1 | **Yes** | `HandleMRInfoFailure` sends MERGE_FAILED for conflicts + test failures |
| 6 | P1 | **Yes** | `createConflictResolutionTaskForMR` calls `MergeSlotEnsureExists` + `MergeSlotAcquire` |
| 7 | P1 | **Yes** | Go code replaces prose merge instructions |
| 8 | P1 | **Yes** | Go code uses `mr.Target` directly — no LLM substitution |
| 9 | P2 | No (deferred) | Needs sling.go changes |
| 10 | P2 | No (deferred) | General formula concern |
| 13 | P2 | **Yes** | E2E tests exercise the conflict pipeline |
| 14 | P2 | **Yes** | Configurable merge strategy (rebase-ff default, squash opt-in) |
| 15 | P3 | **Yes** | This IS the plan — wire dead code into production |

**Coverage: 9 of 12 pre-existing issues addressed.** 3 deferred (dispatch, sling routing,
test diagnosis) — all require changes outside the Engineer/refinery scope.

---

## Success Criteria

- [ ] `gt refinery process-next` processes an MR and merges to main (exit 0)
- [ ] `gt refinery process-next` processes an MR and merges to integration branch (exit 0)
- [ ] Conflict creates task with `## Metadata` format, blocks MR, acquires slot (exit 1)
- [ ] Test failure rejects MR, notifies witness, reopens issue (exit 2)
- [ ] Empty queue returns exit 3
- [ ] Source issue left open when merging to integration branch
- [ ] Source issue closed when merging to default branch
- [ ] Both merge strategies tested (rebase-ff and squash)
- [ ] E2E test covers full lifecycle: create → work → process-next → land
- [ ] E2E test covers conflict: process → conflict → resolve → process again
- [ ] Formula updated to call command instead of raw git (optional, can defer)
- [ ] All existing tests pass (`go test ./...`)
