# Ambiguity Analysis

## Summary

The PRD is well-structured with clear user stories and explicit acceptance criteria, making it above-average in precision. However, several dependency-type semantics are used without definition, the staged-status format remains an acknowledged open question that blocks multiple downstream decisions, and several acceptance criteria use terms ("orphan", "parked rig", "capacity") whose operational definitions are either missing or contradicted by the Non-Goals section. The most impactful ambiguities are around dependency-type semantics (which affect wave computation correctness), error-recovery behavior during partial Wave 1 dispatch, and the interaction between re-staging and already-launched convoys.

## Findings

### Critical Gaps / Questions

**1. `conditional-blocks` and `waits-for` dependency types are undefined.**

Used in: US-001 AC-5, US-004 AC-1/AC-2, FR-3, FR-4.

The PRD treats `blocks`, `conditional-blocks`, and `waits-for` as first-class dependency types for DAG construction and wave computation, but never defines how `conditional-blocks` or `waits-for` differ from `blocks`. These are not standard beads vocabulary visible elsewhere in the codebase documentation.

- **Why this matters:** Wave computation correctness depends on whether all three types create hard execution edges. If `waits-for` is a soft dependency (proceed anyway after timeout), wave assignment changes entirely. If `conditional-blocks` only applies under certain conditions, the wave algorithm needs condition-evaluation logic — a significant design change.
- **Suggested question:** What are the exact semantics of `conditional-blocks` and `waits-for`? Do both create hard execution edges identical to `blocks`, or do they have weaker guarantees? If conditional, what evaluates the condition?

**2. Staged-status format is architecturally blocking (acknowledged but unresolved).**

Open Question Q1 explicitly calls this out: `staged_ready` with underscores, hyphens, or colons? The PRD itself says this is "architecturally blocking" and that the daemon guard, FR-6, and "most integration tests depend on the answer."

- **Why this matters:** An implementer cannot write FR-6 (`bd create --type=convoy --status=staged_ready`), the daemon guard (Technical Considerations), `ensureKnownConvoyStatus` updates, or `validateConvoyStatusTransition` logic until this is resolved. This blocks US-002, US-003, US-007, US-008, and all integration tests.
- **Suggested question:** What is the decided status format: `staged_ready` (underscore), `staged-ready` (hyphen), or `staged:ready` (colon)? Does `bd doctor` need to be updated, or should the format conform to existing regex?

**3. Re-staging a launched (open) convoy: undefined behavior.**

US-007 AC-7 says "re-staging an existing convoy-id re-analyzes and updates the status." FR-8 says "re-staging must update status and re-compute waves without creating a duplicate." But neither addresses what happens when the convoy is already `open` (launched) with in-flight polecats.

- **Why this matters:** If a user runs `gt convoy stage <already-open-convoy>`, should it: (a) error because the convoy is launched, (b) re-analyze and potentially downgrade status back to `staged_ready` (killing in-flight work?), or (c) re-analyze but keep the convoy open? Two engineers would implement this differently.
- **Suggested question:** Can an already-launched (`open`) convoy be re-staged? If yes, what happens to in-flight polecats dispatched from Wave 1? If no, should `gt convoy stage` reject open/closed convoy IDs?

**4. Partial Wave 1 dispatch failure: convoy state undefined.**

US-008 AC-5 says "if a Wave 1 dispatch fails, continues to next task." But no AC or FR defines:
- What is the convoy's final status if some Wave 1 tasks dispatched and others failed?
- How does the user learn which tasks failed?
- Does the daemon still feed Wave 2 tasks when Wave 1 is only partially dispatched?
- Can the user retry failed Wave 1 dispatches?

- **Why this matters:** Partial dispatch creates a state where the convoy is `open` but some Wave 1 tasks never got polecats. The daemon's `isIssueBlocked` checks blockers — but the failed tasks aren't blocked, they just failed to spawn. The daemon may never retry them, creating a permanently stalled convoy.
- **Suggested question:** After partial Wave 1 dispatch failure, what is the expected recovery path? Should failed dispatches be retried automatically, reported for manual retry, or something else?

**5. Input type detection for mixed-input rejection (FR-10) is unspecified.**

FR-10 says "mixed input types (e.g., epic ID + task IDs in the same invocation) must be detected and rejected." But all inputs are bead IDs — the command receives strings like `hq-abc-12345`.

- **Why this matters:** To detect "mixed" input, the implementation must look up each bead's type (epic vs task vs convoy) before processing. This means FR-10 requires N bead lookups before the main DAG walk begins. The PRD doesn't specify whether type detection is eager (look up all inputs first) or lazy (detect during DAG construction). It also doesn't specify what happens with sub-epic IDs mixed with task IDs — are sub-epics "epics" for this check?
- **Suggested question:** Should mixed-input detection be based on bead `type` field? Are sub-epics considered "epics" for this validation? Is a single convoy ID mixed with anything else always an error?

### Important Considerations

**6. "Orphan detection" has two disjoint definitions ORed together.**

US-003 AC-1 defines orphans as "tasks not reachable from the epic's descendant tree, OR tasks with no blocking deps from any other staged task (isolated in the wave graph)."

The first condition (not reachable from descendant tree) can only occur if the convoy tracks beads outside the epic hierarchy — which seems contradictory to US-001's DAG construction (which only collects descendants). The second condition (no blocking deps = isolated) would flag every independent Wave 1 task. This seems overly broad.

- **Why this matters:** Implementers will argue about whether "isolated in the wave graph" means "has zero blocking deps within the staged set" (which flags all Wave 1 leaf tasks) or "has zero deps of any kind including parent-child" (which is different).
- **Suggested question:** For orphan detection in epic-input mode: can the staged set ever contain beads that aren't descendants of the epic? If not, the first condition is vacuous. For the second condition, does "isolated" mean zero execution-edge deps, or truly disconnected from all other beads?

**7. "Capacity estimation" warning contradicts Non-Goals.**

US-003 AC-5 requires "capacity estimation: number of polecats needed per wave vs available capacity." Non-Goals explicitly says "Capacity plumbing (`isRigAtCapacity` callback) — not yet wired into feeder." Technical Considerations say wave computation is "informational."

- **Why this matters:** Without `isRigAtCapacity`, how does the staging command know "available capacity"? Is it a static config value? A count of running polecats? The PRD says to show this as "informational" but doesn't define the data source.
- **Suggested question:** What is the source of "available capacity" for the informational warning? Is it a static config per rig, a runtime count, or should this AC be deferred with the capacity plumbing?

**8. `gt convoy -i` interactive TUI: exists or future?**

US-009 AC-4 says to print a hint: `gt convoy -i` for interactive TUI monitoring. This implies the TUI exists.

- **Why this matters:** If the TUI doesn't exist yet, the hint is misleading. If it does exist, it's not referenced anywhere else in the PRD as prior art.
- **Suggested question:** Does `gt convoy -i` already exist? If not, should the hint be omitted or phrased as "coming soon"?

**9. "Missing integration branches on sub-epics" warning is vague.**

US-003 AC-2: "Missing integration branches on sub-epics (warn, don't block)." What is an "integration branch"? How is its presence or absence detected? Is it a git branch, a beads field, or something else?

- **Why this matters:** An implementer has no way to implement this warning without knowing what an integration branch is and where to look for it.
- **Suggested question:** What is an "integration branch" in this context? Is it a field on the sub-epic bead, a git branch naming convention, or something else? How does the staging command detect whether it's missing?

**10. "Parked or unavailable target rigs" — undefined concept.**

US-003 AC-3 warns about "parked or unavailable target rigs." The PRD doesn't define what makes a rig "parked" or how availability is checked.

- **Why this matters:** Without a definition, this warning cannot be implemented. Is it a rig status field? A check for running witness processes? A config flag?
- **Suggested question:** How is rig "parked" or "unavailable" status determined? Is there an existing mechanism, or does this need new infrastructure?

### Observations

**11. "Informational only" wave computation vs. actionable Wave 1 dispatch.**

Technical Considerations says wave computation is "informational only" and "display only at stage time." But US-008 says "dispatches all Wave 1 tasks." This isn't a contradiction (staging displays waves; launching acts on Wave 1), but the "informational only" language could mislead an implementer into thinking waves don't drive any behavior.

**12. FR-2 SDK approach vs `bd show` in US-001 AC-1.**

US-001 AC-1 says "`gt convoy stage <bead-id>` runs `bd show` on each bead and errors if any don't exist." FR-2 says "must use the Go SDK (`beads.List(ListOptions{Parent: rootID})`) recursively." These suggest different approaches — `bd show` (CLI subprocess) vs. Go SDK (in-process). FR-2 is probably authoritative (with US-001 being loose language), but the inconsistency could confuse.

**13. `parent-child` deps for hierarchy only — but what types create them?**

US-001 AC-6 says "`parent-child` deps are recorded for hierarchy display but do NOT create execution edges." Clear enough, but the PRD doesn't specify how parent-child relationships are discovered. Is it `bd dep list --type=parent-child`? Or inferred from the recursive `beads.List(Parent: rootID)` walk? Both?

**14. "Should" vs "must" usage is generally consistent.**

The PRD uses "must" in FRs and ACs, and softer language ("should", "may") in Technical Considerations and Non-Goals. This is appropriate and not a significant ambiguity — the boundaries are clear.

**15. US-010 AC-3 vs AC-5: launch on already-staged convoy skips analysis, but re-staging re-analyzes.**

`gt convoy launch <convoy-id>` with `staged_ready` status: "no re-analysis needed." But `gt convoy stage <convoy-id>` (US-007 AC-7): "re-analyzes and updates the status." This is internally consistent but worth noting — the two commands have different behaviors on the same input. An implementer should be aware that `launch` on a staged convoy is a status transition only, while `stage` on a staged convoy is a full re-analysis.

## Confidence Assessment

**Medium-High.** The PRD is unusually thorough — explicit acceptance criteria, named files, reuse callouts, and a dedicated "codebase impact" section. The main ambiguities cluster around: (1) dependency-type semantics that appear to be assumed knowledge from the existing codebase, (2) an explicitly unresolved open question (staged status format) that blocks most implementation, and (3) several warning-type ACs that reference concepts (parked rigs, integration branches, capacity) without operational definitions. Once Q1 (status format) is resolved and dependency-type semantics are clarified, the PRD would be implementation-ready with only minor clarifications needed.
