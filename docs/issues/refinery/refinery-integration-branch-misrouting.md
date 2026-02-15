# Refinery Integration Branch Misrouting

**Date:** 2026-02-15
**Epic:** GT-m9oy (Fix integration branch auto-detection in gt done / refinery flow)
**Affected epic:** GT-8ry (Implement doctor-selectivity)
**Integration branch:** `integration/implement-doctor-selectivity`

## Summary

During the first use of integration branches with the gastown rig, the refinery
repeatedly merged polecat work to `main` instead of the integration branch.
This happened across two separate sling rounds despite different mitigation
attempts. Manual intervention was required each time to reset `origin/main`
back to `upstream/main`.

## Timeline

### Round 1: GT-8ry.1 (Wave 1 — category restructuring)

1. `gt sling GT-8ry.1 gastown` — correctly detected integration branch
2. Polecat furiosa completed work, ran `gt done`
3. `gt done` submitted MR (GT-k2nv) with `target: main` instead of integration branch
4. Refinery merged to main and pushed (`236a788e`)
5. **Root cause:** `gt done` resolved `issueID` to `GT-taay` (a wisp/molecule root)
   instead of `GT-8ry.1` (the base bead). `GT-taay` didn't exist in the main Dolt
   database, so `DetectIntegrationBranch` failed silently and fell back to `main`.
6. **Fix:** Force-pushed `origin/main` back to `upstream/main`. Fast-forwarded
   integration branch to include the commit.

### Round 2: GT-8ry.2, GT-8ry.3, GT-8ry.4 (Wave 2 — parallel)

1. Slung all three with `--no-merge` flag to prevent refinery processing
2. Polecats furiosa, nux, slit all spawned and worked correctly
3. Polecats pushed their work to the integration branch (correct behavior)
4. **However:** MR beads were still created and the refinery received MERGE_READY
   notifications despite `--no-merge`
5. Refinery picked up nux's MR (GT-gyah) and merged it to main (`5e24e795`)
6. Refinery was about to process furiosa's MR (GT-i2as) when stopped via `gt nudge`
7. **Fix needed:** Reset `origin/main` back to `upstream/main` again.

## Root Causes

### 1. hook_bead identity confusion (GT-m9oy.1)

The agent bead's `hook_bead` field is overwritten from the base bead ID to the
molecule/wisp root ID during `gt sling`. When `gt done` falls back to reading
`hook_bead` (because branch name parsing didn't yield an issue ID), it gets
the wisp root. The wisp only exists on the polecat's Dolt branch, not the main
database, so `bd.Show()` fails.

**Code path:**
- `polecat_spawn.go:737` — sets `HookBead = GT-8ry.1` (correct)
- Something during molecule bonding overwrites it to the wisp root
- `done.go:1036` — `getIssueFromAgentHook()` returns the wisp root
- `beads/integration.go:189` — `bd.Show(wispRoot)` fails
- `done.go:552-555` — error silently swallowed, target defaults to `main`

### 2. Silent error swallowing (GT-m9oy.2)

`DetectIntegrationBranch` failures are silently discarded in `done.go:552-555`.
No log output, no warning. The polecat has no indication that it submitted the
MR to the wrong branch.

### 3. Integration branch not forwarded from sling to done (GT-m9oy.3)

`gt sling` successfully detects the integration branch and passes it as a
`base_branch` formula variable (for git rebase). But `gt done` must independently
re-detect it. The detection succeeds at sling time (using the correct base bead
ID against the rig's beads database) but fails at done time (using the wrong
wisp ID against the polecat's Dolt branch).

### 4. --no-merge flag doesn't prevent MR submission (observed in Round 2)

The `--no-merge` flag on `gt sling` was expected to prevent the refinery from
processing the work. While the polecats correctly pushed to the integration
branch, MR beads were still created and the refinery received MERGE_READY
notifications. The refinery then processed these MRs and merged to main.

This suggests `--no-merge` only affects where the polecat pushes its branch,
not whether an MR is submitted to the merge queue.

### 5. Refinery has no awareness of integration branches

The refinery formula (`mol-refinery-patrol`) merges MRs to main unconditionally.
It does not check whether the source issue belongs to an epic with an integration
branch. Even if the MR bead had `target: integration/implement-doctor-selectivity`,
the refinery's merge logic may not honor it — it hardcodes merge-to-main behavior.

## State After Incidents

### What needs fixing (immediate)

- `origin/main` is at `5e24e795` (has nux's list subcommand commit). Needs reset
  to `upstream/main` (`fc426ef0`).

### What is correct

- `origin/integration/implement-doctor-selectivity` has all 7 commits:
  1. `b9f74b65` — spec
  2. `d0c41078` — acceptance criteria
  3. `ab7ed65b` — spec review fixes
  4. `236a788e` — category restructuring (furiosa, Wave 1)
  5. `0fd6bd87` — list subcommand (nux, Wave 2)
  6. `db310989` — filter logic (furiosa, Wave 2)
  7. `00c9fa39` — non-TTY support (slit, Wave 2)

## Filed Beads

- **GT-m9oy** (P1 epic) — Fix integration branch auto-detection in gt done / refinery flow
  - **GT-m9oy.1** (P1 bug) — hook_bead overwritten from base bead to wisp root
  - **GT-m9oy.2** (P1 bug) — gt done silently swallows DetectIntegrationBranch errors
  - **GT-m9oy.3** (P2 task) — Pass integration branch from sling to done explicitly
  - **GT-m9oy.4** (P2 task) — Formula should pass --issue to gt done

## Lessons Learned

1. **Integration branches are not yet safe for autonomous use.** The refinery
   does not understand them. Manual intervention is required for every merge.

2. **`--no-merge` does not prevent MR submission.** It only affects the polecat's
   push target. The merge queue still receives notifications and the refinery
   still processes them.

3. **The refinery must be parked or stopped** when using integration branches
   until the detection bugs are fixed. Otherwise it will merge work to main.

4. **Silent failures are dangerous.** The `DetectIntegrationBranch` error being
   swallowed meant no one knew the MR was targeting the wrong branch until
   after the refinery had already pushed to main.

## Recommended Workaround

Until GT-m9oy is resolved, when using integration branches:

1. **Park the rig** before slinging integration branch work
2. Have polecats push to the integration branch directly (they already do this
   when the worktree is sourced from the integration branch)
3. Skip the refinery entirely — manually verify and close beads
4. Or: fix GT-m9oy.4 (one-line formula change) as a quick mitigation
