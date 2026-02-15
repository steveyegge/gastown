# Refinery Issues

> Known issues with the current formula-driven refinery. Full analysis and
> evidence in the [e2e investigation](../e2e/issue-tracking.md).

## The Core Problem

The refinery formula has Claude execute merge mechanics (git rebase, merge,
push, notifications, bead state) inline as prose instructions. This is
unreliable because:

- Claude must **remember values** (branch names, polecat names, MR IDs) across
  formula steps — one forgotten variable breaks the lifecycle
- Git operations can **fail silently** — the formula can't reliably detect a
  failed push
- The Engineer struct has a **complete Go implementation** of the merge pipeline
  (`engineer.go`) but it's dead code — never called from any production path

The [proposed v2](../concepts/refinery-v2.md) wires the Engineer into three
deterministic commands (`gt refinery prepare/merge/reject`).

---

## Conflict Resolution Is Non-Functional

Three compounding issues make conflict resolution broken end-to-end:

**Infinite retry loop** (e2e #1, P0) — When the formula detects a conflict, it
creates a task and skips the MR, but nothing prevents the MR from re-entering
the ready queue next cycle. Each 30-second patrol cycle retries the same MR and
creates another duplicate task. Unbounded bead pollution.

**Format mismatch** (e2e #2, P0) — The formula creates conflict tasks with
prose metadata, but `mol-polecat-conflict-resolve` expects structured metadata
with list markers. Even if a polecat is dispatched, it can't extract the branch
name or conflict SHA.

**No auto-dispatch** (e2e #4, P1) — No patrol agent scans for conflict tasks.
They sit in `bd ready` indefinitely until a human runs `gt sling` manually.

---

## LLM-Dependent Merge Path

Four issues stem from Claude executing merge steps as prose:

**Merge-push sequence** (e2e #7, P1) — Claude must run checkout → ff-merge →
push → verify → send MERGED → close MR → delete branch in order. Any dropped
step causes silent lifecycle breakage (worktrees accumulate, beads left open).

**Branch substitution** (e2e #8, P1) — Claude substitutes branch names from
formula prose into git commands. One wrong variable means merging the wrong code.

**MERGE_FAILED silent on conflicts** (e2e #5, P1) — The Witness is never
notified when a conflict occurs — only test failures trigger MERGE_FAILED.

**Merge slot in dead code** (e2e #6, P1) — The merge slot mechanism (prevents
duplicate conflict tasks) exists in the Engineer but isn't reachable.

---

## Latent Risks

**Wrong sling formula** (e2e #9, P2) — `gt sling` auto-applies
`mol-polecat-work` for all beads. When conflict dispatch is added, it must
route to `mol-polecat-conflict-resolve` instead.

**Unstructured gate failures** (e2e #10, P2) — The diagnosis seam (branch
regression vs pre-existing) is intentionally LLM-driven, but without
structured gate output Claude works from raw logs.

**LLM inbox parsing** (e2e #12, P2) — Claude parses `gt mail inbox` and must
remember polecat names across steps. Forgetting means MERGED notification
fails.

**Pipeline never exercised** (e2e #13, P2) — Zero conflict tasks exist in
production. The entire conflict resolution pathway is theoretical.

**Merge strategy divergence** (e2e #14, P2) — Formula uses `rebase + ff-only`;
Engineer's `doMerge` uses `squash`. If wired up, strategies would conflict.

---

## Integration Branch Targeting

**`gt done` targets main despite integration branch worktree** (new) — When
`gt sling` creates a polecat worktree from an integration branch, `gt done`
walks the parent chain to find the target. If any bead in the chain is missing,
the walk falls back to `main`. Observed in production: `GT-taay` didn't exist
in the database, breaking the chain `GT-taay → GT-8ry.1 → GT-8ry → integration
branch`.

---

## Engineer Implementation Bugs

Bugs in the Engineer's existing (dead) code that need fixing when wiring it
into production. Found during [GT-o9l research](../e2e/issue-tracking.md).

- **Nil guards** — `HandleMRInfoFailure` doesn't guard against nil `mr` or nil
  `sourceIssue`, panics on malformed MR beads (~5 line fix)
- **Missing syncCrewWorkspaces** — `MergeMR` doesn't call
  `syncCrewWorkspaces()` after successful merge (~1 line fix)
- **Post-push race** — Verifies against local ref immediately after push;
  on slow networks the ref may not have updated yet (~5 line fix)
- **Missing ClaimMR** — `Prepare` doesn't call `ClaimMR()` to prevent
  concurrent processing of the same MR (~3 line fix)

---

## Fixed (PR #1226)

These were fixed by xexr's integration branch PR:

- ~~`gt mq integration status` reports 0 MRs~~ (e2e #3) — queries by Label
  instead of Type
- ~~`auto_land` enforcement LLM-only~~ (e2e #11) — pre-push hook added
- ~~`makeTestMR` unrealistic beads~~ (e2e #16) — uses correct Type + Labels
- ~~FORBIDDEN directives untestable~~ (e2e #17) — pre-push hook tested
