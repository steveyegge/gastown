# Convoy Stability Roadmap

How to get from where we are to the target UX, while preserving existing
workflows and fixing the reliability problems people actually hit.

---

## Current state

Two branches ready, neither merged to upstream/main:

| Branch | Status | What it does |
|--------|--------|-------------|
| `feat/convoy-manager-rewrite` | PR #1615 open | Replaces daemon event detection, consolidates observers, adds auto-dispatch |
| `feat/convoy-feeder-phase1` | 1 commit on top of rewrite | Adds type filtering, blocks checking, capacity plumbing, iteration, staged statuses |

Three PRDs written:

| PRD | Status | Scope |
|-----|--------|-------|
| Phase 1 (feeder redesign) | Implemented | Type filtering, blocks deps, capacity, iteration, staged statuses |
| Phase 2 (stage/launch) | Designed | `gt convoy stage`, `gt convoy launch`, epic status management, wave display |
| Phase 3 (advanced dispatch) | Designed | FeederStrategy interface, coordinator polecat, depth validation, auto-formula |

---

## Workflows to preserve

### Workflow A: Manual bead creation + batch sling

The most common pattern today:

```
bd create --type=task "Fix auth timeout"       → sh-task-1
bd create --type=task "Add validation"         → sh-task-2
bd create --type=task "Integration tests"      → sh-task-3
bd dep add sh-task-2 sh-task-1 --type=blocks
gt sling sh-task-1 sh-task-2 sh-task-3 gastown
```

What happens today:
- Batch sling creates 3 separate auto-convoys (one per task)
- Each task gets its own `"Work: <title>"` convoy
- No convoy groups the 3 tasks together
- Tasks sling in parallel (all at once), not sequentially
- `blocks` deps are ignored by the feeder — sh-task-2 gets slung even
  though sh-task-1 hasn't finished

What people expect:
- Tasks dispatch in dependency order
- Tasks that are blocked don't get slung until their blockers close
- Completed tasks land on the target branch through the refinery

### Workflow B: design-to-beads + manual sling

```
/design-to-beads PRD.md
→ creates: root epic, sub-epics, leaf tasks
→ adds: parent-child deps (organizational hierarchy)
→ adds: blocks deps (execution ordering between tasks)
gt sling <task1> <task2> <task3> gastown
```

Same outcome as Workflow A: 3 separate auto-convoys, blocks deps ignored.
The epic and sub-epic structure exists in beads but has no effect on
dispatch or completion tracking.

### Workflow C: Manual convoy creation

```
gt convoy create "Auth overhaul" sh-task-1 sh-task-2 sh-task-3
gt sling sh-task-1 gastown
→ witness feeds sh-task-2 when sh-task-1 closes (serial)
→ witness feeds sh-task-3 when sh-task-2 closes (serial)
→ convoy auto-closes when all 3 are done
```

This works on upstream/main but is serial (one task at a time) and the
witness feed ignores blocks deps, type filters, and rig capacity.

---

## Target UX

The ideal experience, achievable at the end of this roadmap:

```
/design-to-beads PRD.md
→ creates: root epic → sub-epics → leaf tasks
→ adds: parent-child (hierarchy) + blocks (ordering) deps
→ sub-epics get integration branches

gt convoy stage <epic-id>
→ walks DAG, validates structure, displays route plan (tree + waves)
→ creates staged convoy tracking all beads

gt convoy launch <convoy-id>
→ activates convoy, dispatches Wave 1 tasks
→ daemon feeds subsequent waves as tasks close
→ sub-epic status auto-managed (open → in_progress → closed)
→ when sub-epic closes: sling sub-epic with review formula
→ review formula examines accumulated changes on integration branch
→ on approval: integration branch lands to main/parent branch
→ convoy closes when root epic closes
```

---

## What people actually report as broken

The most common complaint: **tasks don't make it through the refinery and
land on the target branch.** This is NOT a convoy problem — it's a
sling→done→refinery pipeline reliability problem. The convoy system layers
on top of this pipeline.

### Critical failure points (independent of convoys)

| # | Failure | Where | Severity | Recovery |
|---|---------|-------|----------|----------|
| 1 | Dolt branch merge fails after MR bead creation | `done.go:808-819` | Critical | MR bead stranded on dead Dolt branch. Invisible to refinery. No automated recovery. |
| 2 | Push fails (all 3 tiers) | `done.go:531-572` | Critical | Commits local-only. Worktree preserved. Manual recovery required. |
| 3 | MR bead creation fails | `done.go:744-752` | High | Branch pushed but no MR. Witness notified. No auto-recovery. |
| 4 | Refinery never wakes (agent stall) | Agent-level | High | Heartbeat restarts, but gap can be minutes. |
| 5 | Merge conflict blocks MR indefinitely | `engineer.go:764-786` | Medium | Conflict task must be dispatched + resolved. Stalls if rig at capacity. |
| 6 | Orphaned MR (branch deleted, MR still open) | `engineer.go:1086-1198` | Medium | Anomaly detection finds it. Agent must act. |

These failures affect ALL polecat work, not just convoy-tracked work.
Fixing them benefits the entire system.

### Convoy-specific failure points

| # | Failure | Fixed by |
|---|---------|----------|
| 7 | Blocked tasks get slung (blocks deps ignored) | Phase 1 (`isIssueBlocked`) |
| 8 | Epics get slung to polecats (no type filter) | Phase 1 (`IsSlingableType`) |
| 9 | Cross-rig close events invisible to daemon | Rewrite (multi-rig SDK polling) |
| 10 | Daemon doesn't feed next task after close | Rewrite (continuation feeding) |
| 11 | Refinery convoy check passes wrong path (never works) | Rewrite (call removed) |
| 12 | First dispatch failure abandons entire convoy | Phase 1 (iteration past failures) |
| 13 | Stranded scan is reporting-only, doesn't auto-dispatch | Rewrite (`feedFirstReady`) |

---

## Merge decision: what goes into PR #1615

The rewrite PR without Phase 1 still dispatches epics, ignores blocks deps,
and gives up after the first failure. A reviewer testing the rewrite will
hit these bugs immediately if their convoy tracks anything other than
simple, independent, unblocked tasks.

### Recommendation: merge Phase 1 items 1, 2, 4 into the rewrite PR

| Phase 1 item | Merge into rewrite? | Rationale |
|-------------|---------------------|-----------|
| Type filtering (`IsSlingableType`) | **Yes** | Without it, the rewrite's new auto-dispatch actively makes things worse — it now eagerly slings epics that the old system only reached via the slow Deacon patrol path |
| Blocks dep checking (`isIssueBlocked`) | **Yes** | Same reasoning — the rewrite's event-driven feed now dispatches blocked tasks within 5 seconds instead of the old witness-delay. Faster dispatch of blocked work is a regression |
| Iteration past dispatch failures | **Yes** | The rewrite's `feedFirstReady` tries `ReadyIssues[0]` only. If that issue targets a parked rig, the convoy is stuck for 30s until the next stranded scan. Trivial fix, high impact |
| Capacity callback plumbing | **No** | Just interface plumbing. The actual callback isn't implemented. Adds parameter noise to every function without runtime benefit yet |
| Staged statuses | **No** | No command creates them. Pure forward prep for Phase 2. Can be added when Phase 2 starts |

This means cherry-picking the type filter, blocks check, and iteration
changes from the Phase 1 commit into the rewrite branch, leaving the
capacity plumbing and staged statuses for a separate PR.

**Alternative: merge the entire Phase 1 commit into the rewrite PR.** The
capacity plumbing is nil-defaulted everywhere (no behavioral change when
nil) and the staged statuses just expand the validation function. The diff
is ~2000 lines but most of it is tests and the PRD doc. The code changes
are ~200 lines across 3 files. This is simpler than cherry-picking.

---

## Phased plan

### Milestone 0: Land the foundation (PR #1615 + Phase 1)

**Goal:** Fix the convoy dispatch infrastructure so existing workflows
work correctly.

**What ships:**
- Multi-rig event polling (fixes cross-rig blindness)
- Continuation feeding after close events (fixes daemon-doesn't-feed)
- Stranded scan auto-dispatch (fixes reporting-only limitation)
- Observer consolidation (removes broken refinery call, removes witness
  coupling)
- Type filtering (prevents slinging epics)
- Blocks dep checking (prevents slinging blocked tasks)
- Iteration past dispatch failures (prevents stuck convoys)
- Process group isolation (prevents orphaned subprocesses)

**What it fixes for Workflow A:**
- `gt sling <task1> <task2> <task3>` still creates 3 auto-convoys, but
  each convoy now correctly checks blocks deps before dispatch. A blocked
  task won't be slung until its blocker closes. BUT: each convoy only
  tracks 1 task, so the blocks check only matters if the daemon's
  stranded scan or event poll somehow reaches across convoys (it doesn't —
  the blocks check runs on the tracked issue itself, which queries the
  issue's deps in the beads store, so it works regardless of convoy
  boundaries).
- Type filtering prevents accidental epic slinging.

**What it does NOT fix for Workflow A:**
- Batch sling still creates N separate auto-convoys. No group convoy.
- No convoy-level dependency ordering (each convoy is independent).
- Tasks still sling in parallel if there's rig capacity.

**What it fixes for Workflow B:**
- Same as Workflow A. design-to-beads creates blocks deps, which are now
  respected by the feeder.

**What it fixes for Workflow C:**
- Manual convoys now get reliable daemon-driven feeding with blocks
  checking. The witness removal doesn't matter because the daemon
  now handles everything the witness did (and more).

**Action items:**
1. Decide: cherry-pick Phase 1 items into rewrite, or squash-merge the
   whole Phase 1 commit into the rewrite branch
2. Update PR #1615 description to reflect the combined changes
3. Run test suite: `go test ./...` including integration tests
4. Submit for review

### Milestone 1: Pipeline reliability (independent of convoys)

**Goal:** Fix the sling→done→refinery pipeline failures that cause
"tasks don't land" complaints.

This is the highest-impact work for user-reported problems. Convoys
can't deliver if the underlying pipeline drops tasks.

**Work items:**

| # | Problem | Proposed fix | Complexity |
|---|---------|-------------|------------|
| 1a | Dolt branch merge fails → MR stranded | Add retry loop in `done.go` for `MergePolecatBranch`. If all retries fail, store MR bead ID in a recovery file and notify witness with `RECOVERY_NEEDED` signal. | Medium |
| 1b | No recovery for stranded MR beads | Daemon-level scan: periodically check for MR beads on non-main Dolt branches. If found, attempt merge. If merge succeeds, nudge refinery. | Medium |
| 1c | Refinery agent stall | Harden refinery heartbeat. Add a daemon-level MR queue monitor that nudges (or restarts) the refinery when MRs sit unprocessed beyond a threshold. | Medium |
| 1d | Merge conflicts block indefinitely | Track conflict task age. If unresolved after N hours, escalate to Mayor/owner with the specific conflict details. | Low |

**This milestone is independent of convoy work.** It can be done in
parallel by a different contributor, or sequenced after Milestone 0.

### Milestone 2: Stage and launch (`gt convoy stage`, `gt convoy launch`)

**Goal:** Enable the `/design-to-beads → gt convoy stage → gt convoy
launch` workflow.

**Depends on:** Milestone 0 (the feeder must respect blocks deps and
filter types for staged convoys to work correctly).

**What ships (from Phase 2 PRD):**
- `gt convoy stage <bead-id>` — DAG walking, validation, wave computation,
  tree + wave route plan display
- `gt convoy launch <convoy-id>` — activates convoy, dispatches Wave 1
- Epic status management (open → in_progress → closed)
- Integration branch awareness (warnings when missing)
- Staged status transitions (staged:ready ↔ staged:warnings → open)

**Key design decisions already made:**
- `parent-child` is organizational only, never blocking (aligned with
  `bd ready` and beads SDK)
- Execution ordering is via explicit `blocks` deps
- Wave computation is informational (display only), runtime dispatch uses
  per-cycle `isIssueBlocked` checks
- Integration branch creation and landing remain manual (or refinery
  auto-land)

**What this enables for Workflow B:**
```
/design-to-beads PRD.md
gt convoy stage <root-epic-id>
→ see tree view + wave view
→ see warnings (missing integration branch, parked rigs, etc.)
gt convoy launch <convoy-id>
→ Wave 1 tasks dispatched automatically
→ subsequent waves fed by daemon as tasks close
→ epic statuses update as children progress
→ convoy closes when root epic closes
```

**What it does NOT enable yet:**
- Sub-epic review formula (see Milestone 3)
- Auto-formula detection for epic slinging (Phase 3)
- Coordinator polecat (Phase 3)

### Milestone 3: Sub-epic review gate

**Goal:** When all tasks under a sub-epic complete and merge into the
sub-epic's integration branch, automatically trigger a comprehensive
review of the accumulated changes before landing.

This is the missing piece between "tasks merge to integration branch"
and "integration branch lands to main."

**Current state:** Integration branch landing is purely mechanical — all
children closed + all MRs merged = ready to land. There is no review
step that examines the combined diff.

**Proposed mechanism:**

1. **Sub-epic completion trigger**: When the convoy's epic status
   management (Milestone 2 US-014) closes a sub-epic, instead of (or
   before) auto-landing, sling the sub-epic itself with a review formula.

2. **Review formula**: A new formula (e.g., `mol-integration-review` or
   adapt `code-review.formula.toml`) that:
   - Checks out the integration branch
   - Computes the full diff against the base branch
   - Reviews the accumulated changes for:
     - Cross-task consistency
     - API contract violations between tasks
     - Missing tests for combined functionality
     - Merge conflict residue
   - Produces a review report
   - If approved: runs `gt mq integration land <sub-epic-id>`
   - If rejected: creates a fix task, blocks the sub-epic on it

3. **Convoy awareness**: The convoy stays open while the review runs.
   The review polecat's completion triggers the next sub-epic (if the
   root epic has `blocks` deps between sub-epics) or the root epic
   closure.

**Integration points:**
- `internal/convoy/operations.go` — after closing an epic, check if it
  has an integration branch. If yes, sling with review formula instead of
  calling `gt mq integration land`.
- `internal/daemon/convoy_manager.go` — the event poll detects the
  review polecat's bead close, feeds the next sub-epic or closes the
  root epic.
- New formula: `mol-integration-review.formula.toml`

**design-to-beads changes needed:**
- Ensure sub-epics get integration branches (either design-to-beads
  creates them, or `gt convoy stage` creates them at stage time)
- Ensure `blocks` deps exist between sub-epics if sequential ordering
  is desired

### Milestone 4: Advanced dispatch (Phase 3 PRD)

**Goal:** Pluggable dispatch strategies and coordinator polecats.

**What ships:**
- `FeederStrategy` interface
- Hierarchy depth validation (opt-in)
- Auto-generate `blocks` deps from hierarchy (`--infer-blocks`)
- Auto-formula detection in `gt sling` (epic → coordinator formula)
- Coordinator polecat strategy
- Dynamic DAG decomposition

This milestone is the furthest out and the least urgent. The default
dispatch strategy (Phase 1 feeder with blocks checking) covers the
common case. The coordinator polecat is for complex epics where
AI-driven task selection outperforms static dependency ordering.

---

## Dependency graph

```
Milestone 0: Foundation
  (convoy-manager-rewrite + phase1)
  │
  ├──────────────────────────┐
  │                          │
  v                          v
Milestone 1: Pipeline    Milestone 2: Stage/Launch
  (done/refinery fixes)    (gt convoy stage/launch)
  │                          │
  │                          v
  │                      Milestone 3: Sub-epic review gate
  │                          │
  └──────────┬───────────────┘
             │
             v
         Milestone 4: Advanced dispatch
```

Milestones 1 and 2 are independent and can run in parallel.
Milestone 3 depends on Milestone 2 (needs epic status management).
Milestone 4 depends on both 2 and 3 being stable.

---

## What design-to-beads needs to change

The current design-to-beads plugin creates the right structure (epics
with parent-child deps, tasks with blocks deps). For the staged convoy
workflow, it needs:

| Change | When needed | Who |
|--------|------------|-----|
| Create `blocks` deps between sub-epics (not just between tasks) | Milestone 2 | design-to-beads plugin |
| Create integration branches for sub-epics | Milestone 3 | design-to-beads plugin or `gt convoy stage` |
| Output the root epic ID for `gt convoy stage` input | Milestone 2 | design-to-beads plugin |

The current plugin already creates blocks deps between tasks. The gap is
inter-sub-epic ordering: if Sub-Epic A should complete before Sub-Epic B
starts, a `blocks` dep between them (or between A's last task and B's
first task) must exist.

If design-to-beads doesn't create inter-sub-epic blocks deps, `gt convoy
stage` will show them dispatching in parallel (Wave 1), which may or may
not be desired. The `--infer-blocks` flag (Milestone 4) can auto-generate
these from creation order, but explicit deps from the PRD structure are
more reliable.

---

## Summary: what to do next

1. **Now:** Decide whether to merge all of Phase 1 into PR #1615 or
   cherry-pick the safety-critical items (type filter, blocks check,
   iteration). Whole-commit merge is simpler.

2. **Now:** Update PR #1615, run tests, submit for review.

3. **Next:** Start Milestone 1 (pipeline reliability) and/or Milestone 2
   (stage/launch) depending on priorities. Milestone 1 has broader impact
   (fixes "tasks don't land" for everyone). Milestone 2 enables the
   staged convoy UX.

4. **After 2:** Milestone 3 (sub-epic review gate) is the key piece
   connecting design-to-beads output to the full automated workflow.

5. **Later:** Milestone 4 (advanced dispatch) when the common case is
   stable.
