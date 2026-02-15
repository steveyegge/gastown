# Refinery Patrol Design

> Implementation analysis of the current formula-driven merge pipeline.

## How It Works Today

The Refinery patrol formula (`mol-refinery-patrol`, v6) contains 12 steps.
Claude executes each step by following prose instructions that include
inline git commands, bead operations, and mail sends.

### Formula Steps

| Step | Title | What Claude does |
|------|-------|-----------------|
| inbox-check | Check refinery mail | Parse `gt mail inbox`, extract MERGE_READY fields, track in memory |
| queue-scan | Scan merge queue | `git fetch --prune`, `gt mq list`, verify branches exist |
| process-branch | Mechanical rebase | `git checkout -b temp`, `git rebase`, handle conflicts inline |
| run-tests | Run quality checks | Execute setup/typecheck/lint/build/test commands |
| handle-failures | Handle failures | Diagnose regression vs pre-existing, reject or proceed |
| merge-push | Merge and push | `git merge --ff-only`, `git push`, verify, notify, close, cleanup |
| loop-check | Check for more | Loop back or proceed |
| generate-summary | Handoff summary | Write patrol cycle summary |
| check-integration | Check integration branches | Land ready epics if `auto_land=true` |
| context-check | Assess session health | RSS, age, cognitive state |
| patrol-cleanup | Inbox hygiene | Archive stale messages |
| burn-or-loop | End-of-cycle | Squash wisp and loop or handoff |

### The Engineer (Dead Code)

The `Engineer` struct in `internal/refinery/engineer.go` contains a complete
implementation of the merge pipeline in Go:

- `ProcessMR()` / `ProcessMRInfo()` — full MR processing
- `doMerge()` — git merge with strategy support
- `HandleMRInfoSuccess()` — post-merge bookkeeping
- `HandleMRInfoFailure()` — conflict/failure handling with task creation
- `createConflictResolutionTaskForMR()` — structured conflict task creation
- `ListReadyMRs()` — queue query with blocker filtering

**None of these methods are called from any production code path.** The
formula bypasses the Engineer entirely. See
[Refinery Issues](../../issues/refinery.md) for the implications.

## Known Failure Modes

The formula-driven approach has several documented failure modes. See
[Refinery Issues](../../issues/refinery.md) for the full list. The key
themes are:

- **LLM memory dependency** — Claude must remember branch names, polecat
  names, and MR IDs across formula steps
- **Silent failures** — git push can fail without the formula detecting it
- **Infinite retry loops** — conflict MRs re-enter the queue each cycle
- **Format mismatches** — conflict tasks created with prose that the resolver
  can't parse
- **Dead code** — the Engineer solves most of these problems but isn't wired up

## Proposed Changes

See [Refinery Patrol Design v2](./refinery-v2.md) for the three-command
pipeline proposal that addresses these issues by wiring the Engineer's
methods into `gt refinery prepare/merge/reject`.

## Key Files

| Component | File |
|-----------|------|
| Patrol formula | `internal/formula/formulas/mol-refinery-patrol.formula.toml` |
| Engineer (dead code) | `internal/refinery/engineer.go` |
| Refinery manager | `internal/refinery/manager.go` |
| Refinery commands | `internal/cmd/refinery.go` |

## Related Design Docs

- [Watchdog Chain](../watchdog-chain.md) — how the daemon ensures the Refinery runs
- [Mail Protocol](../mail-protocol.md) — MERGE_READY/MERGED/MERGE_FAILED message flow
