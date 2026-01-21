# Open PR Analysis Report

**Branch**: `refactor/agents-clean`
**Repository**: https://github.com/steveyegge/gastown
**Analysis Date**: 2026-01-18
**Open PRs Analyzed**: 50

---

## Executive Summary

| Category | Count |
|----------|-------|
| FIXED_BY_REFACTOR | 2 |
| DUPLICATED | 0 |
| STILL_NEEDED | 29 |
| OBSOLETED | 2 |
| CONFLICTS | 17 |

**Key Finding**: 17 PRs will likely conflict with our refactored architecture. Most conflicts involve `daemon.go`, manager files, and `start.go` which have been significantly restructured.

---

## CONFLICTS (17 PRs)

These PRs modify code that has been refactored. They will need to be rebased or reimplemented against our new architecture.

### PR #682 - fix(polecat): cleanup orphan sessions during allocation
**Author**: Daniel Sauer | **Files**: `internal/cmd/polecat_spawn.go`

**Conflict Reason**: Adds `cleanupOrphanPolecatState()` to polecat spawn. Our refactor may have changed the spawn entry points.

**Recommendation**: Review if cleanup logic should be integrated into `factory.Start()`.

---

### PR #683 - fix(deacon): improve session stability with startup consistency
**Author**: Daniel Sauer | **Files**: `internal/cmd/deacon.go`

**Conflict Reason**: Modifies deacon startup path. Our refactor centralized startup in factory.go.

**Recommendation**: Verify permission dialog handling and startup fallback are present in factory.Start().

---

### PR #686 - fix(polecat): close open MRs before nuking
**Author**: Daniel Sauer | **Files**: `internal/cmd/polecat.go`

**Conflict Reason**: Minor - adds MR check to polecat.go which still exists but may have changed.

**Recommendation**: Should apply cleanly with minor adjustments.

---

### PR #687 - feat(daemon): add exponential backoff for agent restarts
**Author**: Daniel Sauer | **Files**: `internal/daemon/daemon.go`, `internal/daemon/restart_tracker.go`

**Conflict Reason**: Adds RestartTracker to daemon heartbeat loop. Our refactor may have changed how agents are restarted.

**Recommendation**: Review if backoff should be integrated into factory.Start() or daemon supervision.

---

### PR #688 - feat(daemon): add Mayor to daemon supervision
**Author**: Daniel Sauer | **Files**: `internal/daemon/daemon.go`

**Conflict Reason**: Adds `ensureMayorRunning()` to daemon. Our unified factory architecture handles all agents consistently.

**Recommendation**: May be FIXED_BY_REFACTOR - verify factory.Agents() handles Mayor supervision.

---

### PR #690 - fix(polecat): improve lifecycle handling
**Author**: Daniel Sauer | **Files**: `internal/cmd/done.go`, `internal/polecat/manager.go`, `internal/witness/handlers.go`

**Conflict Reason**: Modifies witness handlers and polecat manager. Our refactor significantly changed agent lifecycle.

**Recommendation**: Review each fix individually - push before nuke, cleanup_status validation, respawn logic.

---

### PR #739 - fix(refinery): reconcile status with actual tmux session state
**Author**: Joshua Vial | **Files**: `internal/refinery/manager.go`, `internal/polecat/namepool.go`

**Conflict Reason**: Modifies refinery manager Status() method. Our refactor removed individual manager Start() methods.

**Recommendation**: Verify tmux session reconciliation is present in current Status() implementation.

---

### PR #746 - Fix critical bug: kill main pane PID
**Author**: Roland Tritsch | **Files**: `internal/tmux/tmux.go`, `internal/tmux/tmux_test.go`

**Conflict Reason**: Critical fix for `getAllDescendants()` - may have already been addressed in our branch (commit 65).

**Recommendation**: **PRIORITY** - Verify this fix is present. Compare with commit 65 (`f89ac47f`).

---

### PR #751 - feat(down/shutdown): detect and kill stray Gas Town Claude processes
**Author**: dwsmith1983 | **Files**: `internal/cmd/down.go`, `internal/cmd/orphans.go`, `internal/cmd/start.go`

**Conflict Reason**: Modifies down.go and start.go which are affected by our refactor.

**Recommendation**: Review if orphan detection logic should be integrated into factory shutdown.

---

### PR #752 - Fix daemon shutdown detection bug
**Author**: Roland Tritsch | **Files**: Unknown

**Conflict Reason**: Daemon changes may conflict with our agent supervision architecture.

**Recommendation**: Need to review specific changes.

---

### PR #759 - fix(shutdown): fix session counter bug and add --cleanup-orphans flag
**Author**: Roland Tritsch | **Files**: `internal/cmd/start.go`

**Conflict Reason**: Significant changes to start.go shutdown logic. Our refactor changed how sessions are killed.

**Recommendation**: Review session counting logic for compatibility.

---

### PR #769 - feat: add initial prompt for autonomous patrol startup
**Author**: dwsmith1983 | **Files**: Unknown

**Conflict Reason**: May modify deacon/witness startup which is now handled by factory.

**Recommendation**: Review for compatibility with unified startup architecture.

---

### PR #770 - feat(patrol): implement PING/PONG protocol with backoff
**Author**: dwsmith1983 | **Files**: Formula files primarily

**Conflict Reason**: Formula file changes - should be straightforward merge but verify.

**Recommendation**: Formula changes generally safe; verify formula paths.

---

### PR #725 - fix(up): add PPID-based orphan cleanup before starting services
**Author**: Atmosphere-Aviation | **Files**: Unknown

**Conflict Reason**: Modifies gt up flow which may be affected by factory changes.

**Recommendation**: Review for compatibility with factory.Start().

---

### PR #729 - fix(daemon): spawn Deacon immediately after killing stuck session
**Author**: aleiby | **Files**: Daemon files

**Conflict Reason**: Daemon spawn logic may conflict with factory-based restarts.

**Recommendation**: Verify immediate respawn behavior is preserved.

---

### PR #745 - Fix deacon patrol process leak
**Author**: Roland Tritsch | **Files**: Unknown

**Conflict Reason**: Process leak fixes may relate to our tmux changes.

**Recommendation**: Review for overlap with commit 65/145.

---

### PR #771 - chore: fix CI build break (lint + formula sync)
**Author**: Julian Knutsen | **Files**: Unknown

**Conflict Reason**: Build/lint fixes - may need rebasing after our refactor.

**Recommendation**: Should be straightforward to rebase.

---

## STILL_NEEDED (29 PRs)

These PRs address issues not covered by our refactor.

### High Priority

| PR | Title | Files | Notes |
|----|-------|-------|-------|
| #689 | fix(tui): add timeout to CombinedSource goroutines | `internal/tui/feed/events.go` | Goroutine leak fix - should merge |
| #733 | Fix updateAgentHookBead directory resolution | `internal/cmd/sling_helpers.go` | Beads fix - should merge |
| #744 | Fix orphan detection for hq-* sessions | `internal/doctor/orphan_check.go` | Doctor check - should merge |
| #755 | Fix tmux error handling for "no current target" | Unknown | Tmux robustness - review |
| #758 | fix(formulas): replace hardcoded ~/gt/ paths | Formula files | Portability fix - should merge |

### Medium Priority

| PR | Title | Files | Notes |
|----|-------|-------|-------|
| #706 | fix(down): move bd daemon kill after tmux sessions | Unknown | Shutdown order - review |
| #709 | feat(config): add role context overrides | Unknown | Config enhancement |
| #710 | feat: simplify issue and PR templates | Templates | Simple merge |
| #711 | feat(config): add refinery disable flag per-rig | Unknown | Config enhancement |
| #713 | fix: make gt agents default to list output | Unknown | UX improvement |
| #714 | perf: parallelize mail inbox queries | Unknown | Performance improvement |
| #715 | fix(doctor): check patrol formulas | Unknown | Doctor improvement |
| #718 | Fix GT_ROOT export for beads formula search | Unknown | Formula fix |
| #719 | Fix settings templates to use 'gt prime --hook' | Unknown | Template fix |
| #728 | feat(upgrade): add gt upgrade command | Unknown | New feature |
| #730 | feat(down): add TTY-based orphan sweep | Unknown | Shutdown improvement |
| #731 | fix(costs): add event to BeadsCustomTypes | Unknown | Config fix |
| #732 | fix(scripts): replace Unicode with ASCII | Scripts | Compatibility |
| #734 | feat(doctor): add stale-beads-redirect check | Unknown | Doctor improvement |
| #737 | Add comprehensive tests for internal/dog | Tests | Test coverage |
| #738 | fix(hooks): display crew-level settings | Unknown | UX improvement |
| #748 | fix(tmux): resolve claude path for alias | Unknown | Path resolution |
| #750 | fix: detect actual rig layout | Unknown | Layout detection |
| #753 | fix: ensure gitignore patterns on role creation | Unknown | Git fix |
| #754 | fix: orphan-processes check only detects GT processes | Unknown | Filter fix |
| #760 | Add Nix flake support | Build files | New build system |
| #765 | fix(config): correct Claude prompt prefix | Unknown | Config fix |

### Low Priority / Feature Requests

| PR | Title | Notes |
|----|-------|-------|
| #707 | fix(install): Makefile uses go install | Install fix |
| #723 | feat: Add multi-model AI agents | Major feature |
| #727 | feat(rig): add manifest support | New feature |
| #740 | fix: ExpectedPaneCommands returns node and claude | Process detection |
| #747 | Add GitHub Copilot CLI as built-in runtime | Major feature |
| #701 | fix(ci): resolve lint warning | Build fix |

---

## FIXED_BY_REFACTOR (2 PRs)

These issues are addressed by our architectural changes.

### PR #688 - feat(daemon): add Mayor to daemon supervision

**Status**: LIKELY FIXED

Our unified factory architecture handles all agents through `factory.Agents()`. The daemon should supervise all agents consistently without special-casing Mayor.

**Verification Needed**: Confirm factory.Agents() returns Mayor and daemon restarts it.

---

### PR #683 - fix(deacon): improve session stability with startup consistency

**Status**: PARTIALLY FIXED

Our factory.Start() centralizes startup logic for all agents. The startup fallback behavior should be consistent.

**Verification Needed**: Confirm permission dialog handling is present in startup path.

---

## OBSOLETED (2 PRs)

These PRs modify code paths that no longer exist.

### PR #690 (Partial) - fix(polecat): improve lifecycle handling

**Obsoleted Part**: The `witness/handlers.go` respawn logic references old handler patterns.

**Still Needed Part**: Push branch before nuke, cleanup_status validation.

---

### PR #739 (Partial) - fix(refinery): reconcile status with actual tmux session

**Status**: Manager architecture changed. Status() method may have moved.

**Verification Needed**: Confirm status reconciliation exists in current code.

---

## Recommendations

### Immediate Actions

1. **Merge PR #689** (TUI goroutine leak) - Independent fix, no conflict
2. **Merge PR #733** (updateAgentHookBead) - Important fix, clean merge likely
3. **Merge PR #744** (hq-* orphan detection) - Doctor improvement
4. **Verify PR #746** (kill main pane PID) - Critical fix, may duplicate commit 65

### Before Merging Refactor

1. Review all CONFLICTS PRs for functionality that should be preserved
2. Create tracking issues for any STILL_NEEDED PRs that block merge
3. Communicate with PR authors about rebase requirements

### After Merging Refactor

1. Close OBSOLETED PRs with explanation
2. Help PR authors rebase CONFLICTS PRs to new architecture
3. Prioritize STILL_NEEDED PRs for merge

---

## PR List by Author

| Author | PRs | Notes |
|--------|-----|-------|
| Daniel Sauer | #682, #683, #686, #687, #688, #689, #690, #701, #758, #765 | Prolific contributor, many conflicts |
| Roland Tritsch | #744, #745, #746, #752, #759 | Shutdown/orphan fixes |
| dwsmith1983 | #751, #753, #754, #769, #770 | Process detection, patrol |
| aleiby | #729, #731, #734, #738 | Daemon and doctor fixes |
| boshu2 | #710, #713, #714 | UX improvements |
| Others | Various | Individual contributions |

---

## Appendix: Full PR List

| # | Title | Author | Created |
|---|-------|--------|---------|
| 771 | chore: fix CI build break | julianknutsen | 2026-01-18 |
| 770 | feat(patrol): implement PING/PONG protocol | dwsmith1983 | 2026-01-18 |
| 769 | feat: add initial prompt for autonomous patrol | dwsmith1983 | 2026-01-18 |
| 765 | fix(config): correct Claude prompt prefix | sauerdaniel | 2026-01-18 |
| 760 | Add Nix flake support | harryaskham | 2026-01-18 |
| 759 | fix(shutdown): fix session counter bug | roland-at-ailtir | 2026-01-18 |
| 758 | fix(formulas): replace hardcoded paths | sauerdaniel | 2026-01-18 |
| 755 | Fix tmux error handling | TechnicallyShaun | 2026-01-18 |
| 754 | fix: orphan-processes check only GT | dwsmith1983 | 2026-01-18 |
| 753 | fix: ensure gitignore patterns | dwsmith1983 | 2026-01-18 |
| 752 | Fix daemon shutdown detection bug | roland-at-ailtir | 2026-01-18 |
| 751 | feat(down/shutdown): detect stray processes | dwsmith1983 | 2026-01-18 |
| 750 | fix: detect actual rig layout | dwsmith1983 | 2026-01-18 |
| 748 | fix(tmux): resolve claude path | ark-tik | 2026-01-18 |
| 747 | Add GitHub Copilot CLI | mzkoch | 2026-01-18 |
| 746 | Fix critical bug: kill main pane PID | roland-at-ailtir | 2026-01-18 |
| 745 | Fix deacon patrol process leak | roland-at-ailtir | 2026-01-18 |
| 744 | Fix orphan detection for hq-* sessions | roland-at-ailtir | 2026-01-18 |
| 740 | fix: ExpectedPaneCommands | jrgifford | 2026-01-18 |
| 739 | fix(refinery): reconcile status | joshuavial | 2026-01-18 |
| 738 | fix(hooks): display crew-level settings | aleiby | 2026-01-18 |
| 737 | Add comprehensive tests for dog | Louisvranderick | 2026-01-18 |
| 734 | feat(doctor): add stale-beads-redirect check | aleiby | 2026-01-18 |
| 733 | Fix updateAgentHookBead directory | julianknutsen | 2026-01-18 |
| 732 | fix(scripts): replace Unicode with ASCII | groblegark | 2026-01-18 |
| 731 | fix(costs): add event custom type | aleiby | 2026-01-18 |
| 730 | feat(down): add TTY-based orphan sweep | Atmosphere-Aviation | 2026-01-18 |
| 729 | fix(daemon): spawn Deacon after kill | aleiby | 2026-01-18 |
| 728 | feat(upgrade): add gt upgrade command | kiwiupover | 2026-01-17 |
| 727 | feat(rig): add manifest support | easel | 2026-01-17 |
| 725 | fix(up): add PPID-based orphan cleanup | Atmosphere-Aviation | 2026-01-17 |
| 723 | feat: Add multi-model AI agents | mindfu23 | 2026-01-17 |
| 719 | Fix settings templates gt prime --hook | roland-at-ailtir | 2026-01-17 |
| 718 | Fix GT_ROOT export | roland-at-ailtir | 2026-01-17 |
| 715 | fix(doctor): check patrol formulas | roland-at-ailtir | 2026-01-17 |
| 714 | perf: parallelize mail inbox queries | boshu2 | 2026-01-17 |
| 713 | fix: make gt agents default to list | boshu2 | 2026-01-17 |
| 711 | feat(config): add refinery disable flag | dwsmith1983 | 2026-01-17 |
| 710 | feat: simplify issue and PR templates | boshu2 | 2026-01-17 |
| 709 | feat(config): add role context overrides | dwsmith1983 | 2026-01-17 |
| 707 | fix(install): Makefile uses go install | easel | 2026-01-17 |
| 706 | fix(down): move bd daemon kill | dwsmith1983 | 2026-01-17 |
| 701 | fix(ci): resolve lint warning | sauerdaniel | 2026-01-17 |
| 690 | fix(polecat): improve lifecycle handling | sauerdaniel | 2026-01-17 |
| 689 | fix(tui): add timeout to goroutines | sauerdaniel | 2026-01-17 |
| 688 | feat(daemon): add Mayor to supervision | sauerdaniel | 2026-01-17 |
| 687 | feat(daemon): add exponential backoff | sauerdaniel | 2026-01-17 |
| 686 | fix(polecat): close MRs before nuking | sauerdaniel | 2026-01-17 |
| 683 | fix(deacon): improve session stability | sauerdaniel | 2026-01-17 |
| 682 | fix(polecat): cleanup orphan sessions | sauerdaniel | 2026-01-17 |
