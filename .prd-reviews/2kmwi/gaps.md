# Missing Requirements

## Summary

The PRD is architecturally thorough — it identifies the right code touchpoints, reuses existing infrastructure, and addresses the core staging/launching workflow well. However, it is almost entirely focused on the *happy path* of stage → launch → daemon feeds. Several production-critical scenarios are unaddressed: concurrent access to staged convoys, cleanup of abandoned staged convoys, interaction with the existing `gt sling` workflow, handling of already-in-progress tasks, and convoy cancellation UX. These gaps are unlikely to cause data loss but will cause confusion, stale state accumulation, and support burden if not addressed before launch.

The most architecturally significant gap is the absence of any staleness detection between staging and launching. The PRD treats the staged wave plan as a snapshot, but provides no mechanism to detect when that snapshot has diverged from reality (beads modified, deps changed, tasks closed externally). A stale launch could dispatch tasks that are already running or violate new dependency constraints.

## Findings

### Critical Gaps / Questions

**1. Stale staged convoy — no invalidation or re-validation at launch time**

The PRD specifies that re-staging updates the convoy (FR-8, US-007 AC-7), but `gt convoy launch` on an already-staged convoy skips re-analysis (US-010 AC-3: "no re-analysis needed if status is `staged_ready`"). Between staging and launching, any of these could happen:
- A tracked bead is closed externally
- New `blocks` deps are added, creating cycles
- A bead's rig prefix is remapped in `routes.jsonl`
- A tracked bead is deleted

**Why this matters:** Launching a stale staged convoy could dispatch already-completed tasks, violate new dependency constraints, or fail silently on deleted beads. The daemon's `isIssueBlocked` check will catch some cases at feed time, but the user gets no warning at launch time.

**Suggested question:** Should `gt convoy launch <convoy-id>` re-validate the DAG by default, with a `--skip-validation` flag for speed? Or at minimum, check that all tracked beads still exist and are in a dispatchable state?

**2. Concurrent staging — race condition on overlapping convoy detection**

The PRD mentions overlap detection (codebase confirms `findOverlappingConvoys()`), but doesn't address the race condition: two concurrent `gt convoy stage` invocations on the same epic can both pass the overlap check before either creates a convoy, resulting in duplicate staged convoys tracking the same beads.

**Why this matters:** Duplicate staged convoys lead to double-dispatch when both are launched. The daemon would feed from both, potentially spawning two polecats for the same task.

**Suggested question:** Should convoy creation use an advisory lock or CAS (compare-and-swap) on the tracked bead set? Or is the operational model "only one human runs staging at a time" sufficient?

**3. Abandoned staged convoy cleanup — no TTL or garbage collection**

The daemon ignores staged convoys by design (correct — they shouldn't be fed). But there's no mechanism to clean up staged convoys that are never launched. A user who runs `gt convoy stage` ten times during iterative design, never launching any, will accumulate ten orphaned staged convoys indefinitely.

**Why this matters:** Stale staged convoys pollute `gt convoy list` output, waste beads storage, and cause confusion when users encounter old staged convoys they don't recognize.

**Suggested question:** Should staged convoys have a TTL (e.g., 24h)? Should the stranded scan detect and warn about old staged convoys? Or is manual `gt convoy close <id>` sufficient?

**4. Interaction with `gt sling` — no mutual exclusion**

The PRD is silent on what happens when a user runs `gt sling <task-id>` on a task that is tracked by a staged convoy. The `gt sling` command creates an auto-convoy via `createBatchConvoy`, meaning the task would be tracked by both the staged convoy and a new auto-convoy.

**Why this matters:** When the staged convoy is launched, the task is already dispatched via sling. The daemon may attempt to re-dispatch it, or the staged convoy may never close because it's waiting on a task that was completed under a different convoy.

**Suggested question:** Should `gt sling` check for staged convoy membership and either refuse (with a message pointing to the staged convoy) or automatically launch the staged convoy? Should `gt convoy stage` mark tracked beads with a label to prevent ad-hoc slinging?

**5. Cancellation UX — no explicit cancel/unstage command**

The transition validation allows `staged_*→closed` (confirmed in codebase), but the PRD defines no user-facing command for this. A user who stages a convoy and decides not to launch has no documented workflow.

**Why this matters:** Without a clear cancel path, users will either: (a) leave staged convoys indefinitely (see gap #3), (b) try `gt convoy close` and hope it works, or (c) ask support how to cancel.

**Suggested question:** Should `gt convoy cancel <convoy-id>` be added as an alias for `gt convoy close` with staged-specific messaging? Or is `gt convoy close <convoy-id>` sufficient with a note in `--help`?

**6. Already-dispatched or in-progress tasks in the staged set**

The PRD doesn't specify behavior when the epic contains tasks that are already `in_progress` (assigned to a polecat) or `closed`. US-008 says "dispatches all Wave 1 tasks" but doesn't clarify whether already-active tasks are skipped, re-dispatched, or cause an error.

**Why this matters:** If a user runs `gt convoy stage <epic>` on an epic where some tasks were manually slung earlier, the wave plan includes tasks that are already running. Launching would either double-dispatch (spawning a second polecat for the same task) or fail silently.

**Suggested question:** Should staging detect and exclude already-dispatched tasks? Should they appear in the wave display with a "skipped: already in_progress" annotation? Should launch refuse if any Wave 1 tasks are already assigned?

### Important Considerations

**7. Cross-rig event propagation for wave progression**

The PRD notes cross-rig beads exist but doesn't address how the daemon handles cross-rig close events for wave progression. The `ConvoyManager` polls per-rig stores — if task A (rig alpha) blocks task B (rig gastown), and task A closes, the gastown daemon's event poll needs to detect the close in rig alpha's store. The `CheckConvoysForIssue` function queries HQ beads (where convoys live), but close events are emitted from rig-level stores.

**Why this matters:** Wave 2 tasks that depend on Wave 1 tasks in other rigs may not be fed promptly (or at all) if the daemon doesn't poll the correct stores.

**Suggested question:** Does the daemon already poll all rig stores, or only its own? If only its own, how are cross-rig blocking deps resolved at runtime?

**8. `gt convoy status` display for staged convoys**

The PRD mentions printing `gt convoy status <convoy-id>` as a follow-up hint (US-009) but doesn't specify how the existing `gt convoy status` command should display staged convoy information. Staged convoys have wave data, warnings, and a "not yet launched" state that differs from open convoys.

**Why this matters:** Users who check on a staged convoy with the existing status command may see confusing or incomplete information if the display doesn't account for the staged state.

**Suggested question:** Should `gt convoy status` show wave breakdown, warnings, and a "launch with `gt convoy launch`" hint for staged convoys?

**9. `gt convoy list` filtering changes**

The PRD doesn't mention how `gt convoy list` should handle staged convoys. Currently it presumably shows open and closed convoys. Should staged convoys appear by default? With a filter flag?

**Why this matters:** If staged convoys appear in the default list alongside open convoys, users may confuse them. If they're hidden, users may forget about them.

**Suggested question:** Should `gt convoy list` show staged convoys by default, or require `--include-staged`? Should there be a `--status` filter flag?

**10. Partial launch failure reporting and recovery**

US-008 says "if a Wave 1 dispatch fails, continues to next task." But the PRD doesn't specify:
- What error information is shown for failed dispatches
- Whether the convoy status is still `open` after partial failure
- How the user retries failed dispatches
- Whether the daemon's stranded scan will eventually pick up undispatched tasks

**Why this matters:** A partial launch failure leaves the convoy in an ambiguous state. The user needs clear guidance on what succeeded, what failed, and what to do next.

**Suggested question:** Should launch output clearly list failed dispatches with error reasons? Should the daemon's stranded scan handle retry automatically? Should `gt convoy launch` be idempotent (safe to re-run after partial failure)?

**11. Convoy size limits and performance bounds**

No mention of maximum convoy size. Epic DAG walking is recursive and in-memory. Wave computation is topological sort. For a realistic upper bound: 100 tasks? 500? The PRD should set expectations.

**Why this matters:** A user running `gt convoy stage` on an epic with 200 descendant tasks could experience multi-second delays or memory issues if the implementation isn't bounded.

**Suggested question:** What's the expected maximum convoy size? Should staging impose a limit (e.g., 100 tasks) with `--no-limit` override?

### Observations

**12. `--json` + `--launch` combination not specified**

Can `gt convoy stage --json --launch` be used? If so, what does the JSON output look like after a successful launch (includes dispatched polecats?) vs. after a launch failure (includes error details?)? The PRD only describes `--json` for staging output.

**13. Re-staging a launched convoy**

The transition validation blocks `open→staged_*`, which is correct. But the PRD doesn't explicitly state this constraint in the user stories. A user who launched a convoy and wants to "re-stage" (add more tasks, recompute waves) has no documented path.

**14. Daemon feeding after launch: Wave 1 only vs. daemon discovery**

The PRD says "subsequent waves are NOT dispatched — the daemon feeds them as Wave 1 tasks close." This is correct but relies on the daemon's existing `feedNextReadyIssue` + `isIssueBlocked` logic. The PRD should note that wave numbers are display-only — the daemon doesn't use wave metadata, it independently evaluates blocking deps per issue. This is an important mental model clarification.

**15. Testing strategy absent**

Quality gates are defined (go vet, go build, go test), but there's no testing strategy. What test fixtures are needed? How are beads store interactions mocked? Are integration tests expected? The codebase already uses specific patterns (`BdCmd().WithAutoCommit()`) — the PRD should reference these.

**16. Open Question 1 appears resolved in code**

The PRD lists Q1 (status format: underscores vs. colons vs. hyphens) as OPEN. However, the codebase already implements `staged_ready` and `staged_warnings` as underscore-separated statuses with `isStagedStatus` checking the `staged_` prefix. The PRD should mark Q1 as RESOLVED if the implementation has been accepted.

## Confidence Assessment

**Medium-High confidence.** The PRD covers the core workflow well — the staging pipeline, wave computation, error/warning detection, and launch mechanics are all specified. The gaps are primarily in operational edge cases (concurrent access, stale state, cleanup, interaction with existing commands) and UX completeness (cancel, list, status display). None of the gaps are architecturally blocking — they can be addressed as follow-up user stories — but several (stale launch, concurrent staging, sling interaction) could cause production incidents if not addressed before widespread adoption.
