# E2E Investigation: Issue Tracking

> Issues identified during the [e2e integration branch investigation](./initial-e2e-investigation.md), sorted by severity.
> See investigation doc Sections 2-4 for full analysis and evidence.
>
> **Cross-referenced against PR #1226** (xexr, `feat/integration-branch-enhancement`) on 2026-02-13.
> Julian's manual testing found 4 bugs; xexr's 28/28 test run confirmed all fixed.
>
> **Updated 2026-02-13** to reflect PR #1419 (`refinery/stability-testability`), which wires the
> Engineer's dead code into three deterministic commands (`gt refinery prepare/merge/reject`),
> resolving the conflict resolution pipeline and LLM reliability gaps identified below.

---

## Resolution History

### PR #1226 (xexr) — Integration branch support

Of the 17 issues identified, **4 were fixed by xexr**, **3 were improved**.

#### Fixed by xexr in PR #1226

| # | Issue | Fix | Evidence |
|---|-------|-----|----------|
| 3 | `gt mq integration status` reports 0 MRs | `Label: "gt:merge-request"` replaces `Type: "merge-request"` in both `findOpenMRsForIntegration` and `runMqIntegrationStatus` | Julian's Bug 1; confirmed fixed in 28/28 test run |
| 11 | `auto_land` FORBIDDEN enforcement LLM-only | Pre-push hook added with deterministic integration branch ancestry detection. Blocks pushes to default branch containing integration content unless `GT_INTEGRATION_LAND=1`. Formula also has FORBIDDEN directives. | Julian's Bug 3; `.githooks/pre-push` +39 lines |
| 16 | `makeTestMR` creates unrealistic beads | `makeTestMR` now creates `Type: "task"` with `Labels: ["gt:merge-request"]`. Mock `List` has label filtering. Regression tests `TestMakeTestMR_RealisticFields` and `TestMockBeadsList_LabelFilter` added. | `mq_testutil_test.go`, `mq_integration_test.go` (+911 lines) |
| 17 | FORBIDDEN directives untestable in Go | `.githooks/pre-push_test.sh` (221 lines) tests the deterministic pre-push guardrail. The LLM-level FORBIDDEN is defense-in-depth; the hook is the enforceable layer, and it's now tested. | `.githooks/pre-push_test.sh` (new file) |

Also fixed by xexr (Julian's bugs not in our original list):
- **Bug 2**: Epic auto-closed before land → `runMqIntegrationLand` now handles `epicAlreadyClosed` gracefully
- **Bug 4**: Duplicate `gt mq integration create` → checks epic metadata for existing branch, `resolveUniqueBranchName` disambiguates collisions with numeric suffix

### PR #1419 (l0g1x) — Refinery stability & testability

Wired the Engineer's dead merge code (Issue #15) into three deterministic CLI commands, replacing
LLM-dependent formula steps with Go code. This was identified in the original investigation as the
**highest-leverage fix** (see Backlog section below).

#### Fixed by l0g1x in PR #1419

| # | Issue | Fix | Evidence |
|---|-------|-----|----------|
| 1 | Infinite retry loop / duplicate conflict tasks | `Prepare()` handles conflicts internally: calls `HandleMRInfoFailure` which creates a conflict task via `createConflictResolutionTaskForMR`, then blocks the MR on that task via `beads.AddDependency`. Blocked MRs don't appear in `ListReadyMRs`, breaking the retry loop. | `engineer.go:692-700`, `engineer.go:1093-1104` |
| 2 | Format incompatibility (creation vs consumption) | Conflict task is now created programmatically by `createConflictResolutionTaskForMR` with structured metadata (branch, target, source issue, conflict SHA). No more prose ↔ structured format mismatch. | `engineer.go:1117-1133` |
| 5 | MERGE_FAILED protocol silent on conflicts | `HandleMRInfoFailure` now sends `MERGE_FAILED` with `failureType: "conflict"` for all failure types including conflicts, not just test failures. | `engineer.go:1075-1089` |
| 6 | Merge slot setup only in dead code | `createConflictResolutionTaskForMR` is now called from `HandleMRInfoFailure` (which is called from `Prepare`). Merge slot acquisition happens before task creation. No longer dead code. | `engineer.go:1093-1094` |
| 7 | LLM-dependent merge-push sequence | `gt refinery merge <mr-id>` performs the entire merge-push sequence deterministically: checkout target, ff-merge, push, verify, notify, close beads, delete branch. Formula calls the command instead of instructing Claude to run git steps. | `engineer.go:723-768`, `refinery.go:981` |
| 8 | LLM-dependent branch substitution | Commands read MR fields (branch, target) directly from beads via `FindMR`. No formula variable substitution needed — the Engineer resolves branches from the MR bead itself. | `engineer.go:1500-1513`, `refinery.go:993-997` |
| 14 | Merge strategy divergence (formula vs Engineer) | `doMerge` now supports both `"rebase-ff"` (default, matches formula) and `"squash"` via `config.MergeStrategy`. No more divergence — same strategies available to both paths. | `engineer.go:315-347` |
| 15 | Engineer merge methods are dead code | Methods wired into `gt refinery prepare`, `gt refinery merge`, `gt refinery reject` commands. `Prepare` → `MergeMR` → `HandleMRInfoSuccess` and `Prepare` → `RejectMR` → `HandleMRInfoFailure` are now reachable production code paths. 24 tests cover these methods. | `refinery.go:932-1060`, `engineer_merge_test.go` (24 tests) |

#### Improved by l0g1x (not fully resolved)

| # | Issue | Improvement | Remaining gap |
|---|-------|-------------|---------------|
| 10 | LLM-dependent test failure diagnosis | Quality gate failures are now structured (`PrepareResult.GateFailed`, `GateError`). The formula's `accept-or-reject` step receives structured output, not raw logs. | Claude still decides if failure is branch regression vs pre-existing. This is an intentional design choice — the "diagnosis seam" keeps human-like judgement in the loop. |
| 13 | Conflict resolution pipeline never exercised | All pipeline stages (conflict detection → task creation → MR blocking → MERGE_FAILED notification) are now covered by unit tests in `engineer_merge_test.go`. | Zero conflict tasks in production beads DB. E2E exercised in tests only. |

### Still pre-existing (backlog)

| # | Severity | Issue | Notes |
|---|----------|-------|-------|
| 4 | **P1** | No agent auto-dispatches conflict tasks | Conflict tasks are now created correctly (#1, #2, #6 fixed) and MRs are blocked (#1 fixed), but no patrol agent auto-slings conflict tasks. Task sits in `bd ready` until manual dispatch. Architectural gap — needs a dispatcher role or formula step. |
| 9 | **P2** | `gt sling` auto-applies wrong formula | Pre-existing auto-apply logic (Issue #288). When dispatch is added for conflict tasks, sling must route to `mol-polecat-conflict-resolve` not `mol-polecat-work`. |
| 12 | **P2** | LLM-dependent inbox parsing | Formula still has Claude parse inbox output. The `prepare` command handles the merge queue directly, but inbox-check for new MRs remains formula-driven. |

---

## Full Issue Table (All 17)

| # | Severity | Status | Issue | Impact | Affected Code |
|---|----------|--------|-------|--------|---------------|
| 1 | ~~P0~~ | **Fixed** (PR #1419) | ~~**Infinite retry loop / duplicate conflict tasks**~~ — `Prepare()` handles conflicts internally, creates task, blocks MR via dependency. Blocked MRs exit the ready queue. | ~~Unbounded bead pollution~~ | `engineer.go` (Prepare, HandleMRInfoFailure) |
| 2 | ~~P0~~ | **Fixed** (PR #1419) | ~~**Format incompatibility between conflict task creation and consumption**~~ — Conflict task created programmatically with structured metadata by `createConflictResolutionTaskForMR`. | ~~Polecat fails at load-task~~ | `engineer.go` (createConflictResolutionTaskForMR) |
| 3 | ~~P0~~ | **Fixed** (PR #1226) | ~~`gt mq integration status` reports 0 MRs~~ — Fixed: queries by Label instead of Type. | ~~Non-functional~~ | `mq_integration.go` |
| 4 | **P1** | Pre-existing | **No agent auto-dispatches conflict tasks** — Task sits in `bd ready` indefinitely. Manual `gt sling` with explicit formula required. | Conflict resolution requires human intervention | All patrol formulas |
| 5 | ~~P1~~ | **Fixed** (PR #1419) | ~~**MERGE_FAILED protocol silent on conflicts**~~ — `HandleMRInfoFailure` sends `MERGE_FAILED` with `failureType: "conflict"` for all failure types. | ~~No agent aware of conflicts~~ | `engineer.go` (HandleMRInfoFailure) |
| 6 | ~~P1~~ | **Fixed** (PR #1419) | ~~**Merge slot setup only in dead code**~~ — `createConflictResolutionTaskForMR` called from `HandleMRInfoFailure` → `Prepare`. No longer dead code. | ~~Polecat may error~~ | `engineer.go` (createConflictResolutionTaskForMR) |
| 7 | ~~P1~~ | **Fixed** (PR #1419) | ~~**LLM-dependent merge-push sequence**~~ — `gt refinery merge` performs entire sequence deterministically. | ~~Silent lifecycle breakage~~ | `engineer.go` (MergeMR), `refinery.go` |
| 8 | ~~P1~~ | **Fixed** (PR #1419) | ~~**LLM-dependent branch substitution**~~ — Commands read MR fields from beads directly via `FindMR`. No formula variable substitution needed. | ~~Wrong code merged~~ | `engineer.go` (FindMR), `refinery.go` |
| 9 | **P2** | Pre-existing | **`gt sling` auto-applies wrong formula for conflict tasks** — Auto-applies `mol-polecat-work` for bare beads. Matters when automated dispatch is added. | Wrong workflow for conflict tasks | `sling.go:488-494` |
| 10 | **P2** | Improved (PR #1419) | **LLM-dependent test failure diagnosis** — Quality gate failures now structured (`PrepareResult`). Diagnosis seam is intentional — Claude decides branch regression vs pre-existing. | Merges broken code or rejects good code | `engineer.go` (Prepare), formula `accept-or-reject` step |
| 11 | ~~P2~~ | **Fixed** (PR #1226) | ~~`auto_land` FORBIDDEN enforcement LLM-only~~ — Fixed: pre-push hook provides deterministic enforcement with ancestry detection. | ~~Bypasses human review gate~~ | `.githooks/pre-push` |
| 12 | **P2** | Improved (PR #1226) | **LLM-dependent inbox parsing** — Claude must remember values across steps. Parameterization helps but doesn't eliminate dependency. | Polecat worktrees accumulate | `mol-refinery-patrol.formula.toml` (inbox-check) |
| 13 | **P2** | Improved (PR #1419) | **Conflict resolution pipeline never exercised** — Unit-tested in `engineer_merge_test.go` (24 tests). Not yet exercised in production. | Issues undiscoverable until first real conflict | `engineer_merge_test.go` |
| 14 | ~~P2~~ | **Fixed** (PR #1419) | ~~**Merge strategy divergence**~~ — `doMerge` supports both `"rebase-ff"` (default) and `"squash"` via `config.MergeStrategy`. Formula and Engineer now share the same strategy set. | ~~Unexpected history~~ | `engineer.go` (doMerge) |
| 15 | ~~P3~~ | **Fixed** (PR #1419) | ~~**Engineer merge methods are dead code**~~ — Wired into `gt refinery prepare/merge/reject`. 24 tests. | ~~Maintenance burden~~ | `engineer.go`, `refinery.go`, `engineer_merge_test.go` |
| 16 | ~~P3~~ | **Fixed** (PR #1226) | ~~`makeTestMR` creates unrealistic beads~~ — Fixed: uses `Type: "task"` with labels. Regression tests added. | ~~False test confidence~~ | `mq_testutil_test.go` |
| 17 | ~~P3~~ | **Fixed** (PR #1226) | ~~FORBIDDEN directives untestable~~ — Fixed: pre-push hook tested via `.githooks/pre-push_test.sh`. | ~~No regression protection~~ | `.githooks/pre-push_test.sh` |

---

## Summary

| Category | Count | Issues |
|----------|-------|--------|
| **Fixed by xexr** (PR #1226) | 4 | #3, #11, #16, #17 |
| **Fixed by l0g1x** (PR #1419) | 8 | #1, #2, #5, #6, #7, #8, #14, #15 |
| **Improved** | 3 | #10, #12, #13 |
| **Pre-existing (backlog)** | 2 | #4, #9 |

**Of 17 original issues: 12 fixed, 3 improved, 2 remain.**

The conflict resolution pipeline is now functional in code: conflicts are detected, tasks are created with structured metadata, MRs are blocked, and MERGE_FAILED notifications are sent. The two remaining gaps (#4 auto-dispatch, #9 sling routing) are about agent orchestration — the plumbing works, but no patrol agent auto-dispatches conflict tasks yet.

---

## Remaining Backlog

### Agent orchestration (Issues 4, 9)

The conflict resolution pipeline code works end-to-end, but no agent automatically picks up conflict tasks:

- **Issue 4**: Conflict tasks land in `bd ready` but no patrol formula auto-slings them. Needs either a dispatcher step in the refinery patrol formula or a dedicated conflict-dispatch agent.
- **Issue 9**: When dispatch is added, `gt sling` must route conflict tasks to `mol-polecat-conflict-resolve`, not the default `mol-polecat-work`.

### Diagnosis seam (Issue 10 — by design)

The `accept-or-reject` step in the refinery formula is intentionally LLM-driven. Quality gate failures return structured output (`PrepareResult.GateFailed`, `GateError`), but the decision of whether a test failure is a branch regression or pre-existing on the target requires human-like judgement. This is the "diagnosis seam" — the one place where Claude's reasoning adds value over deterministic code.

### Inbox parsing (Issue 12 — low priority)

The formula's inbox-check step still has Claude parse `gt mail inbox` output. This is low-risk because `gt refinery prepare` handles the actual merge queue lookup directly via `ListReadyMRs`. The inbox parsing is for coordination messages, not merge decisions.
