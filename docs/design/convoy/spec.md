# PRD: Convoy Manager

> Daemon-resident event-driven convoy completion and stranded convoy recovery.

**Status**: Implementation complete (all stories DONE)  
**Owner**: Daemon subsystem  
**Related**: [convoy-lifecycle.md](convoy-lifecycle.md) | [testing.md](testing.md) | [convoy-manager.md](../daemon/convoy-manager.md)

---

## 1. Introduction / Overview

Convoys group work but do not autonomously converge on completion unless observers
trigger checks and feed follow-on work. The system now uses a daemon-resident
manager with two loops:

- Event poll loop (`GetAllEventsSince` every 5s) to react to close events.
- Stranded scan loop (`gt convoy stranded --json` every 30s) to recover missed work.

This PRD reformats and clarifies the specification for AI-agent execution,
preserving historical story status while tracking corrective follow-ups explicitly.

---

## 2. Goals

- Ensure convoy completion is event-driven via multi-rig daemon event polling.
- Ensure stranded convoys are recovered without manual intervention.
- Preserve idempotent behavior across repeated observer triggers.
- Make story acceptance criteria explicit and verifiable.
- Track documentation corrections without rewriting delivery history.

---

## 3. Quality Gates

These commands must pass for every implementation story:

- `go test ./...`
- `golangci-lint run`

No browser verification is required for this PRD pass (documentation + backend scope).

---

## 4. User Stories

### US-001: Event-Driven Convoy Completion
**Description:** As a daemon operator, I want closed issues to trigger convoy checks automatically so convoys do not stall waiting for patrol.

**Acceptance Criteria:**
- [x] Event poll runs every 5 seconds using `GetAllEventsSince`.
- [x] `EventClosed` and closed `EventStatusChanged` events trigger convoy check.
- [x] Non-close events do not trigger close-path logic.
- [x] High-water mark advances monotonically.
- [x] Poll errors are logged and retried on next interval.
- [x] Nil store disables event poll loop safely.

### US-002: Stranded Convoy Recovery
**Description:** As a maintainer, I want stranded convoys scanned periodically so ready work is fed and empty convoys are auto-closed.

**Acceptance Criteria:**
- [x] Stranded scan runs on startup, then at configured interval (default 30s).
- [x] Ready convoy issues dispatch via `gt sling <id> <rig> --no-boot`.
- [x] Empty stranded convoys run `gt convoy check <id>`.
- [x] Unknown prefix/rig are skipped with logs, scan continues.
- [x] Dispatch failure on one convoy does not halt scan.

### US-003: Shared Observer Behavior
**Description:** As an observer implementation owner, I want a shared convoy check function so the daemon event poll uses consistent behavior.

**Acceptance Criteria:**
- [x] Shared observer finds `tracks` dependents and excludes `blocks`.
- [x] Already-closed convoys are skipped safely.
- [x] Open convoys run `gt convoy check`.
- [x] If convoy remains open, first ready issue is fed via `gt sling`.
- [x] Wrapper issue IDs (`external:prefix:id`) are normalized.
- [x] Function remains idempotent across duplicate triggers.

### US-004: Witness Observer Integration [REMOVED]
**Description:** Witness convoy observer removed. The daemon's multi-rig event poll
(watching all rig databases + hq) provides event-driven coverage. The stranded scan
(30s) provides backup. The witness's core responsibility is polecat lifecycle
management (observing work, confirming merges, cleaning zombies, managing merge
queues) -- convoy tracking is an orthogonal concern that belongs in the daemon.

**History:** Originally had 6 `CheckConvoysForIssueWithAutoStore` call sites in
`handlers.go` (1 post-merge, 5 zombie paths). All were pure side-effect notification
hooks that didn't influence witness control flow. Removed when daemon gained
multi-rig event polling, eliminating the need for redundant observers.

### US-005: Refinery Observer Integration [REMOVED]
**Description:** Refinery convoy observer removed. See S-05 for rationale.

### US-006: Daemon Lifecycle Integration
**Description:** As a daemon operator, I want convoy manager startup/shutdown to be bounded and correctly ordered.

**Acceptance Criteria:**
- [x] Beads store opens at daemon start (or gracefully degrades).
- [x] Resolved `gtPath` and `bdPath` are passed into manager.
- [x] Manager starts after feed curator initialization.
- [x] Manager stops before store closure.
- [x] Stop path completes within bounded time in tests.

### US-007: Convoy Metadata in MR Fields
**Description:** As refinery scheduler logic, I want convoy metadata in MR fields so convoy-aware prioritization prevents starvation.

**Acceptance Criteria:**
- [x] `convoy_id` and `convoy_created_at` parse and format correctly.
- [x] Underscore/hyphen/camel variants are accepted.
- [x] Refinery scoring can consume parsed convoy metadata.

### US-008: Lifecycle Safety Guardrails
**Description:** As a runtime maintainer, I want manager lifecycle methods to be safe under repeated calls.

**Acceptance Criteria:**
- [x] `Stop()` is idempotent.
- [x] `Stop()` before `Start()` is safe.
- [x] `Start()` double-call guard exists (`atomic.Bool` with `CompareAndSwap`).

### US-009: Context-Aware Subprocess Cancellation
**Description:** As an operator, I want subprocess calls tied to context so shutdown does not hang on stuck child processes.

**Acceptance Criteria:**
- [x] Convoy manager subprocesses use `exec.CommandContext`.
- [x] Observer subprocesses accept and propagate context.
- [x] Shutdown remains bounded even if subprocesses hang.
- [x] No orphan child processes remain on cancellation.

### US-010: Observer Resolved Binary Paths
**Description:** As a maintainer, I want observer subprocess execution to use resolved binary paths to avoid PATH-dependent behavior drift.

**Acceptance Criteria:**
- [x] Observer check/dispatch use resolved `gt` path or explicit fallback strategy.
- [x] Callers thread path or resolve at a single safe boundary.
- [x] Daemon integration paths use resolved binary paths.

### US-011: High-Risk Test Gap Closure
**Description:** As QA, I want high blast-radius invariants covered with explicit assertions so regressions are caught early.

**Acceptance Criteria:**
- [x] Multi-ready issue dispatches only first issue.
- [x] Unknown prefix and unknown rig skips are tested.
- [x] Empty `ReadyIssues` with positive count is safe no-op.
- [x] Non-close event path has negative subprocess assertions.

### US-012: Error-Path Test Gap Closure
**Description:** As QA, I want explicit coverage for error handling branches so operational recovery behavior is proven.

**Acceptance Criteria:**
- [x] `gt convoy stranded` failure path is tested.
- [x] Invalid JSON parsing path is tested.
- [x] `findStranded` failure in scan loop is tested.
- [x] Event poll store error retry behavior is tested.

### US-013: Lifecycle Edge-Case Tests
**Description:** As QA, I want lifecycle edge scenarios covered so start/stop behavior remains reliable.

**Acceptance Criteria:**
- [x] Mid-iteration cancellation exits cleanly.
- [x] Mixed ready+empty stranded lists route correctly.
- [x] `Start()` double-call guard test exists.

### US-014: Test Harness Improvements
**Description:** As a test maintainer, I want reduced setup duplication and stronger negative observability so tests are easier to maintain and trust.

**Acceptance Criteria:**
- [x] Shared scan-test mock builder is extracted and reused.
- [x] All mock scripts emit side-effect call logs.
- [x] Assertion-free convoy manager tests are eliminated.

### US-015: Documentation Consistency Sweep
**Description:** As a maintainer, I want convoy design docs to match current code so implementation work starts from accurate context.

**Acceptance Criteria:**
- [x] `docs/design/daemon/convoy-manager.md` reflects SDK poll architecture.
- [x] `docs/design/convoy/testing.md` stale stream/backoff wording corrected.
- [x] `docs/design/convoy/convoy-lifecycle.md` observer/manual-close drift corrected.
- [x] Broken relative links corrected.
- [x] `spec.md` file map and command inventory corrected.

### US-016: Corrective Follow-Up for Completed Stories
**Description:** As a maintainer, I want corrections to completed stories tracked explicitly so historical status remains stable.

**Acceptance Criteria:**
- [x] Corrective notes added to affected completed stories.
- [x] No completed story downgraded only due to documentation drift.
- [x] Line-number brittle references replaced with semantic references where practical.

### US-017: Refinery Root-Path Verification
**Description:** As a maintainer, I want `CheckConvoysForIssueWithAutoStore` root-path behavior verified in refinery integration to prevent hidden cross-rig visibility bugs.

**Acceptance Criteria:**
- [x] Expected root behavior is documented (town root vs rig path).
- [x] If current behavior is correct, add rationale note near call site/spec.
- [x] If incorrect, create implementation follow-up and link it here -> S-18 in spec.

---

## 5. Functional Requirements

- **FR-1:** The daemon must poll events via SDK every 5s and process close events only.
- **FR-2:** Stranded scan must run independently on a periodic interval and recover missed work.
- **FR-3:** Shared observer logic must remain idempotent and safe under repeated invocation.
- **FR-4:** Witness and refinery integration must call shared observer after completion-relevant events.
- **FR-5:** Manager lifecycle must support safe start/stop under cancellation and repeated calls.
- **FR-6:** Subprocess execution in long-running loops must be context-cancellable.
- **FR-7:** Test suite must cover high-risk invariants, error paths, and lifecycle edge cases.
- **FR-8:** Spec and related design docs must remain internally consistent and code-accurate.

---

## 6. Non-Goals (Out of Scope)

- Convoy owner/requester notification feature expansion (beyond existing behavior).
- Convoy timeout/SLA (`due_at`) design/implementation.
- Explicit `gt convoy reopen` command (implicit reopen via add remains acceptable).
- Test clock injection and broader time-abstraction framework.

---

## 7. Technical Considerations

- Event poll loop and stranded scan loop are intentionally separate goroutines.
- High-water mark state must remain monotonic and race-safe.
- Observer subprocess calls currently use bare `gt` in some paths; path resolution is pending.
- Refinery path semantics (`townRoot` vs rig path) require explicit verification.

---

## 8. Success Metrics

- Convoys close or progress without relying solely on patrol loops.
- Daemon shutdown remains bounded even under subprocess failure scenarios.
- High-priority convoy invariants have direct test coverage.
- Documentation drift findings can be mapped directly to story IDs.
- New agent sessions can execute backlog items without rediscovery.

---

## 9. Open Questions

- ~~Should S-16 be marked DONE now that acceptance criteria are satisfied?~~ **Resolved**: Yes, marked DONE.
- ~~Is refinery `e.rig.Path` the correct root for observer store access in all deployments?~~ **Resolved (S-17)**: No. `e.rig.Path` is a rig path, not town root. Fix tracked in S-18.
- Should S-10 resolve `gt` path at observer entrypoints or thread it from all callers?

---

## 10. Findings-to-Story Mapping

| Finding | Story |
|---------|-------|
| Stream-model doc drift | US-015 |
| Testing doc stale invariants/rows | US-015 |
| Lifecycle observer/manual close drift | US-015 |
| Completed-story corrections needed | US-016 |
| Refinery root-path ambiguity | US-017 |
| Refinery root-path fix (from S-17 finding) | S-18 |

---

## 11. Priority Order

| Priority | Story | Effort | Risk Mitigated |
|----------|-------|--------|----------------|
| **P0** | US-009: Context-aware subprocess cancellation | Small | Shutdown hangs |
| **P0** | US-008: Start() double-call guard | Trivial | Duplicate goroutines |
| **P1** | US-011: High-risk test gap closure | Medium | Invariant regressions |
| **P1** | US-014: Test harness improvements | Medium | Test reliability/maintainability |
| **P1** | US-017: Refinery root-path verification | Small | Cross-rig convoy visibility |
| **P2** | US-010: Observer resolved binary paths | Small | PATH reliability |
| **P2** | US-012: Error-path tests | Small | Recovery behavior confidence |
| **P2** | US-016: Corrective follow-up for completed stories | Small | Historical/spec drift |
| **P3** | US-013: Lifecycle edge-case tests | Small | Additional robustness |
# Convoy Manager Specification

> Daemon-resident event-driven completion and stranded convoy recovery.

**Status**: Implementation complete (all stories DONE)
**Owner**: Daemon subsystem
**Related**: [convoy-lifecycle.md](convoy-lifecycle.md) | [testing.md](testing.md) | [convoy-manager.md](../daemon/convoy-manager.md)

---

## 1. Problem Statement

Convoys group work but don't drive it. Completion depends on a single
poll-based Deacon patrol cycle running `gt convoy check`. When Deacon is down
or slow, convoys stall. Work finishes but the loop never lands:

```
Create -> Track -> Execute -> Issue closes -> ??? -> Convoy closes
```

The gap needs three capabilities:
1. **Event-driven completion** -- react to issue closes, not poll for them.
2. **Stranded recovery** -- catch convoys missed by event-driven path (crash, restart, stale state).
3. **Redundant observation** -- multiple agents detect completion so no single failure blocks the loop.

---

## 2. Architecture

### 2.1 ConvoyManager (daemon-resident)

Two goroutines inside `gt daemon`:

| Goroutine | Trigger | What it does |
|-----------|---------|--------------|
| **Event poll** | `GetAllEventsSince` every 5s, all rig stores + hq | Detects `EventClosed` / `EventStatusChanged(closed)`, calls `CheckConvoysForIssue` |
| **Stranded scan** | `gt convoy stranded --json` every 30s | Feeds first ready issue via `gt sling`, auto-closes empty convoys via `gt convoy check` |

Both goroutines are context-cancellable and coordinate shutdown via `sync.WaitGroup`.

The event poll opens beads stores for all known rigs (via `routes.jsonl`) plus
the town-level hq store. Parked/docked rigs are skipped during polling. Convoy
lookups always use the hq store since convoys are `hq-*` prefixed. Each store
has an independent high-water mark for event IDs.

### 2.2 Shared Observer (`convoy.CheckConvoysForIssue`)

Shared function called by the daemon's event poll:

| Observer | When | Entry point |
|----------|------|-------------|
| **Daemon event poll** | Close event detected in any rig store or hq | `convoy.CheckConvoysForIssue` (hq store passed in) |

The shared function:
1. Finds convoys tracking the closed issue (SDK `GetDependentsWithMetadata` on hq store, filtered by `tracks` type)
2. Skips already-closed convoys
3. Runs `gt convoy check <id>` for open convoys
4. If convoy remains open after check, feeds next ready issue via `gt sling`
5. Idempotent -- safe to call multiple times for the same event

### 2.3 Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| SDK polling (not CLI streaming) | Avoids subprocess lifecycle management, simpler restart semantics |
| High-water mark (atomic int64) | Monotonically advancing, no duplicate event processing |
| One issue fed per convoy per scan | Prevents batch overflow; next issue fed on next close event |
| Stranded scan as safety net | Catches convoys missed by event-driven path (crash recovery) |
| Nil store disables event poll only | Stranded scan still works without beads SDK (degraded mode) |
| Resolved binary paths (PATCH-006) | ConvoyManager resolves `gt`/`bd` at startup to avoid PATH issues |

---

## 3. Stories

### Legend

| Status | Meaning |
|--------|---------|
| DONE | Implemented, tested, integrated |
| DONE-PARTIAL | Implemented but has known gaps |
| TODO | Not yet implemented |

### Quality Gates (for all implementation stories)

These commands must pass for every implementation story in this spec:
- `go test ./...`
- `golangci-lint run`

---

### S-01: Event-driven convoy completion detection [DONE]

**Description**: When an issue closes, the daemon detects the close event via
SDK polling and triggers convoy completion checks.

**Implementation**: `ConvoyManager.runEventPoll` + `pollEvents` in `convoy_manager.go`

**Acceptance criteria**:
- [x] Polls `GetAllEventsSince` on a 5-second interval
- [x] Detects `EventClosed` events
- [x] Detects `EventStatusChanged` where `new_value == "closed"`
- [x] Skips non-close events (close path not triggered)
- [x] Skips events with empty `issue_id`
- [x] Calls `convoy.CheckConvoysForIssue` for each detected close
- [x] High-water mark advances monotonically (no duplicate processing)
- [x] Error on `GetAllEventsSince` logs and retries next interval
- [x] Nil store disables event polling (returns immediately)
- [x] Context cancellation exits cleanly

**Tests**:
- [x] `TestEventPoll_DetectsCloseEvents` -- real beads store, creates+closes issue, verifies log
- [x] `TestEventPoll_SkipsNonCloseEvents` -- create-only, no close detection

**Corrective note**: "Zero side effects" negative assertions have been added via
`TestEventPoll_SkipsNonCloseEvents_NegativeAssertion` (verifies no subprocess
calls, no close detection, and no convoy activity for non-close events). Originally
tracked in S-11; now resolved.

---

### S-02: Periodic stranded convoy scan [DONE]

**Description**: Every 30 seconds, scan for stranded convoys (unassigned work or
empty). Feed ready work or auto-close empties.

**Implementation**: `ConvoyManager.runStrandedScan` + `scan` + `findStranded` + `feedFirstReady` + `closeEmptyConvoy` in `convoy_manager.go`

**Acceptance criteria**:
- [x] Runs immediately on start, then every `scanInterval`
- [x] Calls `gt convoy stranded --json` and parses output
- [x] For convoys with `ready_count > 0`: dispatches first ready issue via `gt sling <id> <rig> --no-boot`
- [x] For convoys with `ready_count == 0`: runs `gt convoy check <id>` to auto-close
- [x] Resolves issue prefix to rig name via `beads.ExtractPrefix` + `beads.GetRigNameForPrefix`
- [x] Skips issues with unknown prefix (logged)
- [x] Skips issues with unknown rig (logged)
- [x] Continues to next convoy after dispatch failure
- [x] Context cancellation exits mid-iteration
- [x] Scan interval defaults to 30s when 0 or negative

**Tests**:
- [x] `TestScanStranded_FeedsReadyIssues` -- mock gt, verify sling log file
- [x] `TestScanStranded_ClosesEmptyConvoys` -- mock gt, verify check log file
- [x] `TestScanStranded_NoStrandedConvoys` -- empty list: asserts sling log absent, check log absent, no convoy activity in logs
- [x] `TestScanStranded_DispatchFailure` -- first sling fails, scan continues
- [x] `TestConvoyManager_ScanInterval_Configurable` -- 0 -> default, custom preserved
- [x] `TestStrandedConvoyInfo_JSONParsing` -- JSON round-trip

---

### S-03: Shared convoy observer function [DONE]

**Description**: A shared function for checking convoy completion and feeding
the next ready issue, callable from any observer.

**Implementation**: `CheckConvoysForIssue` + `feedNextReadyIssue` in `convoy/operations.go`

**Acceptance criteria**:
- [x] Finds tracking convoys via `GetDependentsWithMetadata` filtered by `tracks` type
- [x] Filters out `blocks` dependencies
- [x] Skips already-closed convoys
- [x] Runs `gt convoy check <id>` for open convoys
- [x] After check, if still open: feeds next ready issue via `gt sling`
- [x] Ready = open status + no assignee
- [x] Feeds one issue at a time (first match)
- [x] Handles `external:prefix:id` wrapper format via `extractIssueID`
- [x] Refreshes issue status via `GetIssuesByIDs` for cross-rig accuracy
- [x] Falls back to dependency metadata if fresh status unavailable
- [x] Nil store returns immediately
- [x] Nil logger replaced with no-op (no panic)
- [x] Idempotent (calling multiple times for same issue is safe)
- [x] Returns list of checked convoy IDs

**Tests**:
- [x] `TestGetTrackingConvoys_FiltersByTracksType` -- real store, blocks filtered
- [x] `TestIsConvoyClosed_ReturnsCorrectStatus` -- real store, open vs closed
- [x] `TestExtractIssueID` -- all wrapper variants
- [x] `TestFeedNextReadyIssue_SkipsNonOpenIssues` -- filtering logic
- [x] `TestFeedNextReadyIssue_FindsReadyIssue` -- first match
- [x] `TestCheckConvoysForIssue_NilStore` -- returns nil
- [x] `TestCheckConvoysForIssue_NilLogger` -- no panic
- [x] `TestCheckConvoysForIssueWithAutoStore_NoStore` -- non-existent path, nil

---

### S-04: Witness integration [REMOVED]

**Description**: Witness convoy observer removed. The daemon's multi-rig event
poll (watching all rig databases + hq) provides event-driven coverage for close
events from any rig. The stranded scan (30s) provides backup. The witness's core
job is polecat lifecycle management -- convoy tracking is orthogonal.

**History**: Originally had 6 `CheckConvoysForIssueWithAutoStore` call sites in
`handlers.go` (1 post-merge, 5 zombie cleanup paths). All were pure side-effect
notification hooks. Removed when daemon gained multi-rig event polling.

---

### S-05: Refinery integration [REMOVED]

**Description**: Refinery convoy observer removed. The daemon event poll (5s)
and witness observer provide sufficient coverage. The refinery observer was
silently broken (S-17: wrong root path) for the entire feature lifetime with
no visible impact, confirming the other two observers are sufficient. Since
beads unavailability disables the entire town (not just convoy checks), the
"degraded mode" justification for a third observer does not hold.

**History**: Originally called `CheckConvoysForIssueWithAutoStore` after merge.
S-17 found it passed rig path instead of town root. S-18 fixed it. Subsequently
removed as unnecessary redundancy.

---

### S-06: Daemon lifecycle integration [DONE]

**Description**: ConvoyManager starts and stops cleanly with the daemon.

**Implementation**: Integrated in `daemon.go` `Run()` and `shutdown()` methods.

**Acceptance criteria**:
- [x] Opens beads store at daemon startup (nil if unavailable)
- [x] Passes resolved `gtPath`/`bdPath` to ConvoyManager
- [x] Passes `logger.Printf` for daemon log integration
- [x] Starts after feed curator
- [x] Stops before beads store is closed (correct shutdown order)
- [x] Stop completes within bounded time (no hang)

**Tests**:
- [x] `TestDaemon_StartsManagerAndScanner` -- start + stop with mock binaries
- [x] `TestDaemon_StopsManagerAndScanner` -- stop completes within 5s

---

### S-07: Convoy fields in MR beads [DONE]

**Description**: Merge-request beads carry convoy tracking fields for priority
scoring and starvation prevention.

**Implementation**: `ConvoyID` and `ConvoyCreatedAt` in `MRFields` struct in `beads/fields.go`

**Acceptance criteria**:
- [x] `convoy_id` field parsed and formatted
- [x] `convoy_created_at` field parsed and formatted
- [x] Supports underscore, hyphen, and camelCase key variants
- [x] Used by refinery for merge queue priority scoring

---

### S-08: ConvoyManager lifecycle safety [DONE]

**Description**: Start/Stop are safe under edge conditions.

**Acceptance criteria**:
- [x] `Stop()` is idempotent (double-call does not deadlock)
- [x] `Stop()` before `Start()` returns immediately
- [x] `Start()` is guarded against double-call (`atomic.Bool` with `CompareAndSwap` at `convoy_manager.go:50-51,80-83`)

**Tests**:
- [x] `TestManagerLifecycle_StartStop` -- basic start + stop
- [x] `TestConvoyManager_DoubleStop_Idempotent` -- double stop
- [x] `TestStart_DoubleCall_Guarded` -- second Start() is no-op, warning logged

---

### S-09: Subprocess context cancellation [DONE]

**Description**: All subprocess calls in ConvoyManager and observer
propagate context cancellation so daemon shutdown is not blocked by hanging
subprocesses.

**Implementation**: All `exec.Command` calls replaced with `exec.CommandContext`.
Process group killing via `setProcessGroup` + `syscall.Kill(-pid, SIGKILL)` prevents
orphaned child processes.

**Acceptance criteria**:
- [x] All `exec.Command` calls in ConvoyManager use `exec.CommandContext(m.ctx, ...)` (`convoy_manager.go:200,241,257`)
- [x] All `exec.Command` calls in operations.go accept and use a context parameter
- [x] Daemon shutdown completes within bounded time even if `gt` subprocess hangs (`convoy_manager_integration_test.go:154-206`)
- [x] Killed subprocesses do not leave orphaned child processes (`convoy_manager.go`, `operations.go`)

---

### S-10: Resolved binary paths in operations.go [DONE]

**Description**: Observer subprocess calls use resolved binary paths instead
of bare `"gt"` to avoid PATH-dependent behavior drift.

**Implementation**: `CheckConvoysForIssue` resolves via `exec.LookPath("gt")`
with fallback to bare `"gt"`. Threads `gtPath` parameter to `runConvoyCheck`
and `dispatchIssue` in `operations.go`.

**Acceptance criteria**:
- [x] `runConvoyCheck` and `dispatchIssue` accept a `gtPath` parameter
- [x] `CheckConvoysForIssue` threads resolved path
- [x] All callers updated: daemon (resolved `m.gtPath`)
- [x] Fallback to bare `"gt"` if resolution fails

---

### S-11: Test gap -- priority 1 (high blast-radius invariants) [DONE]

**Description**: Filled testing gaps for core invariants identified in the test
plan analysis.

**Tests added**:

| Test | What it proves |
|------|---------------|
| `TestFeedFirstReady_MultipleReadyIssues_DispatchesOnlyFirst` | 3 ready issues -> sling log contains only first issue ID |
| `TestFeedFirstReady_UnknownPrefix_Skips` | Issue prefix not in routes.jsonl -> sling never called, error logged |
| `TestFeedFirstReady_UnknownRig_Skips` | Prefix resolves but rig lookup fails -> sling never called |
| `TestFeedFirstReady_EmptyReadyIssues_NoOp` | `ReadyIssues=[]` despite `ReadyCount>0` -> no crash, no dispatch |
| `TestEventPoll_SkipsNonCloseEvents_NegativeAssertion` | Asserts zero side effects (no subprocess calls, no convoy activity) |

**Acceptance criteria**:
- [x] All 5 tests passing
- [x] Each test has explicit assertions (no assertion-free "no panic" tests)

---

### S-12: Test gap -- priority 2 (error paths) [DONE]

**Description**: Covered error paths that previously had no test coverage.

**Tests added**:

| Test | What it proves |
|------|---------------|
| `TestFindStranded_GtFailure_ReturnsError` | `gt convoy stranded` exits non-zero -> error returned |
| `TestFindStranded_InvalidJSON_ReturnsError` | `gt` returns non-JSON stdout -> parse error returned |
| `TestScan_FindStrandedError_LogsAndContinues` | `scan()` doesn't panic when `findStranded` fails |
| `TestPollEvents_GetAllEventsSinceError` | `GetAllEventsSince` returns error -> logged, retried next interval |

**Acceptance criteria**:
- [x] All 4 tests passing
- [x] Error messages are verified in log assertions

---

### S-13: Test gap -- priority 3 (lifecycle edge cases) [DONE]

**Description**: Covered lifecycle edge cases identified in the test plan.

**Tests added**:

| Test | What it proves |
|------|---------------|
| `TestScan_ContextCancelled_MidIteration` | Large stranded list + cancel mid-loop -> exits cleanly |
| `TestScanStranded_MixedReadyAndEmpty` | Heterogeneous stranded list routes ready->sling and empty->check correctly |
| `TestStart_DoubleCall_Guarded` | Second `Start()` is no-op, warning logged |

**Acceptance criteria**:
- [x] All 3 tests passing

---

### S-14: Test infrastructure improvements [DONE]

**Description**: Improved test harness quality and reduced duplication.

**Items**:

| Item | Impact |
|------|--------|
| Extract `mockGtForScanTest(t, opts)` helper | Used by 5+ scan tests (`convoy_manager_test.go:57-117`) |
| Add side-effect logger to all mock scripts | All mock scripts write call logs for positive/negative assertions |
| Fix `DispatchFailure` test logger to capture `fmt.Sprintf(format, args...)` | Assertions verify rendered messages with correct IDs |
| Convert `TestScanStranded_NoStrandedConvoys` to negative test | Asserts sling/check logs absent |

**Acceptance criteria**:
- [x] Shared mock builder exists and is used by >= 3 scan tests (5 tests use it)
- [x] All mock scripts write to call log files (negative tests can assert empty)
- [x] No assertion-free tests remain in convoy_manager_test.go

---

### S-15: Documentation update [DONE]

**Description**: Update stale documentation to reflect current implementation.

**Items**:

| Document | Issue |
|----------|-------|
| `docs/design/daemon/convoy-manager.md` | Mermaid diagram shows `bd activity --follow` but implementation uses SDK `GetAllEventsSince` polling |
| `docs/design/daemon/convoy-manager.md` | Text says "Restarts with 5s backoff on stream error" -- no stream, no backoff; it's a poll-retry loop |
| `docs/design/convoy/testing.md` | Row "Stream failure triggers backoff + retry loop" is stale (no stream) |
| `docs/design/convoy/testing.md` | `TestDoubleStop_Idempotent` listed as gap but now exists |
| `docs/design/convoy/convoy-lifecycle.md` | Observer table lists Deacon as primary third observer; implementation uses Refinery |
| `docs/design/convoy/convoy-lifecycle.md` | "No manual close" claim is stale; `gt convoy close --force` exists |
| `docs/design/convoy/convoy-lifecycle.md` | Relative link to convoy concepts doc is broken (`../concepts/...`) |
| `docs/design/convoy/spec.md` | File map test counts drifted from current suite |

**Acceptance criteria**:
- [x] Mermaid diagram shows SDK polling architecture
- [x] Text accurately describes poll-retry semantics
- [x] Testing.md reflects current test inventory
- [x] Lifecycle observer and manual-close sections match implementation
- [x] Broken links in lifecycle doc are fixed
- [x] Spec file-map counts and command list match current source

**Completion note**: Completed in this review pass; remaining ambiguity about
refinery root-path semantics is tracked separately in S-17.

---

### S-16: Corrective follow-up for DONE stories [DONE]

**Description**: Add explicit corrective tasks for inaccuracies discovered in
stories marked DONE, without changing the implementation status itself.

**Rationale**: DONE stories can still contain stale supporting narrative or
inventory details after nearby refactors. Corrections are tracked explicitly
to avoid silently editing historical delivery claims.

**Scope**:
- S-01: clarify that non-close event "zero side effects" is currently partial
  until negative subprocess assertions are added (see S-11)
- S-04: replace brittle line-number call-site references with symbol/section
  anchors in `handlers.go`
- S-05: validate/clarify refinery `townRoot` vs rig-path argument assumptions
  for `CheckConvoysForIssueWithAutoStore`

**Acceptance criteria**:
- [x] Corrective notes are added to affected DONE stories without downgrading status
- [x] S-04 call-site references no longer depend on fixed line numbers
- [x] S-05 includes an explicit note on root-path assumptions and validation status

**Status note**: All corrective notes updated. S-01 negative assertion test now
exists (resolved). S-04 call sites already use semantic descriptions. S-05 note
updated to reflect S-17 verification findings (incorrect path, fix in S-18).

---

### S-17: Refinery observer root-path verification [DONE]

**Description**: Verify whether refinery passing `e.rig.Path` into
`CheckConvoysForIssueWithAutoStore` is correct for convoy visibility.

**Context**:
- Observer helper opens beads store under `<townRoot>/.beads/dolt`
- Refinery currently passes rig path, not explicitly town root

**Findings**:

The current behavior is **incorrect**. `e.rig.Path` is a rig-level path
(`<townRoot>/<rigName>`), set in `rig/manager.go` as `filepath.Join(m.townRoot, name)`.
`OpenStoreForTown` constructs `<path>/.beads/dolt`, so the refinery opens
`<townRoot>/<rigName>/.beads/dolt` instead of `<townRoot>/.beads/dolt`.

The rig-level `.beads/` directory typically contains either a redirect file
(pointing to `mayor/rig/.beads`) or rig-scoped metadata -- not the town-level
Dolt database that holds convoy data. As a result, `beadsdk.Open` either fails
(no `dolt/` directory) or opens a rig-scoped store that does not contain convoy
tracking dependencies. In both cases `CheckConvoysForIssueWithAutoStore` silently
returns nil, effectively **disabling convoy checks from the refinery observer**.

Other observers handle this correctly:
- **Witness**: resolves town root via `workspace.Find(workDir)` before calling
- **Daemon**: passes `d.config.TownRoot` directly

**Fix required**: Resolve town root from `e.rig.Path` using `workspace.Find`
before passing to `CheckConvoysForIssueWithAutoStore`, matching the witness pattern.
See S-18 for implementation.

**Acceptance criteria**:
- [x] Behavioral expectation is documented (town root vs rig root)
- [x] If current behavior is correct, add code comment/spec note explaining why
- [x] If incorrect, create implementation follow-up story and cross-link here -> S-18

---

### S-18: Fix refinery convoy observer town-root path [DONE]

**Description**: Fixed the refinery's `CheckConvoysForIssueWithAutoStore` call to
pass the town root instead of the rig path, so convoy checks actually open the
correct beads store.

**Context**: Identified by S-17 verification. The refinery was passing `e.rig.Path`
(`<townRoot>/<rigName>`) but the function expects the town root. This silently
disabled convoy observation from the refinery.

**Implementation**: `engineer.go` now resolves town root via `workspace.Find(e.rig.Path)`
before calling `CheckConvoysForIssueWithAutoStore`, matching the witness pattern.

**Acceptance criteria**:
- [x] Refinery resolves town root via `workspace.Find(e.rig.Path)` before calling `CheckConvoysForIssueWithAutoStore`
- [x] Pattern matches witness implementation (graceful fallback if town root not found)
- [x] Import `workspace` package added to `engineer.go`
- [x] BUG(S-17) comment in `engineer.go` removed after fix

---

## 4. Critical Invariants

| # | Invariant | Category | Blast Radius | Story | Tested? |
|---|-----------|----------|-------------|-------|---------|
| I-1 | Issue close triggers `CheckConvoysForIssue` | Data | High | S-01 | Yes |
| I-2 | Non-close events produce zero side effects | Safety | Low | S-01 | Yes (`TestEventPoll_SkipsNonCloseEvents_NegativeAssertion`) |
| I-3 | High-water mark advances monotonically | Data | High | S-01 | Implicit |
| I-4 | Convoy check is idempotent | Data | Low | S-03 | Yes |
| I-5 | Stranded convoys with ready work get fed | Liveness | High | S-02 | Yes |
| I-6 | Empty stranded convoys get auto-closed | Data | Medium | S-02 | Yes |
| I-7 | Scan continues after dispatch failure | Liveness | Medium | S-02 | Yes |
| I-8 | Context cancellation stops both goroutines | Liveness | High | S-06 | Yes |
| I-9 | One issue fed per convoy per scan | Safety | Medium | S-02 | Implicit |
| I-10 | Unknown prefix/rig skips issue (no crash) | Safety | Medium | S-02 | Yes (`TestFeedFirstReady_UnknownPrefix_Skips`, `_UnknownRig_Skips`) |
| I-11 | `Stop()` is idempotent | Safety | Low | S-08 | Yes |
| I-12 | Subprocess cancellation on shutdown | Liveness | High | S-09 | Yes (`TestConvoyManager_ShutdownKillsHangingSubprocess`) |

---

## 5. Failure Modes

### Event Poll

| Failure | Likelihood | Recovery | Tested? |
|---------|------------|----------|---------|
| `GetAllEventsSince` error | Low | Retry next 5s interval | Yes (`TestPollEvents_GetAllEventsSinceError`) |
| Beads store nil | Medium | Event poll disabled, stranded scan continues | Yes |
| Close event with empty `issue_id` | Low | Skipped | No |
| `CheckConvoysForIssue` panics | Low | Daemon process crash -> restart | No |

### Stranded Scan

| Failure | Likelihood | Recovery | Tested? |
|---------|------------|----------|---------|
| `gt convoy stranded` error | Low | Logged, skip cycle | Yes (`TestFindStranded_GtFailure_ReturnsError`) |
| Invalid JSON from `gt` | Low | Logged, skip cycle | Yes (`TestFindStranded_InvalidJSON_ReturnsError`) |
| `gt sling` dispatch fails | Medium | Logged, continue to next convoy | Yes |
| `gt convoy check` fails | Low | Logged, continue to next convoy | No |
| Unknown prefix for issue | Low | Logged, skip issue | Yes (`TestFeedFirstReady_UnknownPrefix_Skips`) |
| Unknown rig for prefix | Low | Logged, skip issue | Yes (`TestFeedFirstReady_UnknownRig_Skips`) |
| `gt` subprocess hangs | Low | Context cancellation kills process group | Yes (`TestConvoyManager_ShutdownKillsHangingSubprocess`) |

### Lifecycle

| Failure | Likelihood | Recovery | Tested? |
|---------|------------|----------|---------|
| `Stop()` before `Start()` | Low | `wg.Wait()` returns immediately | No |
| Double `Stop()` | Low | Idempotent | Yes |
| Double `Start()` | Low | Guarded (`atomic.Bool`, no-op) | Yes (`TestStart_DoubleCall_Guarded`) |
| Subprocess blocks shutdown | Low | Context cancellation kills process group | Yes (`TestConvoyManager_ShutdownKillsHangingSubprocess`) |

---

## 6. File Map

### Core Implementation

| File | Contents |
|------|----------|
| `internal/daemon/convoy_manager.go` | ConvoyManager: event poll + stranded scan goroutines |
| `internal/convoy/operations.go` | Shared `CheckConvoysForIssue`, `feedNextReadyIssue`, `getTrackingConvoys`, `IsSlingableType`, `isIssueBlocked` |
| `internal/beads/routes.go` | `ExtractPrefix`, `GetRigNameForPrefix` (prefix -> rig resolution) |
| `internal/beads/fields.go` | `MRFields.ConvoyID`, `MRFields.ConvoyCreatedAt` (convoy tracking in MR beads) |

### Integration Points

| File | How it uses convoy |
|------|-------------------|
| `internal/daemon/daemon.go` | Opens multi-rig beads stores, creates ConvoyManager in `Run()`, stops in `shutdown()` |
| `internal/witness/handlers.go` | Convoy observer removed (S-04 REMOVED) |
| `internal/refinery/engineer.go` | Convoy observer removed (S-05 REMOVED) |
| `internal/cmd/convoy.go` | CLI: `gt convoy create/status/list/add/check/stranded/close/land` |
| `internal/cmd/sling_convoy.go` | Auto-convoy creation during `gt sling` |
| `internal/cmd/formula.go` | `executeConvoyFormula` for convoy-type formulas |

### Tests

| File | What it tests |
|------|--------------|
| `internal/daemon/convoy_manager_test.go` | ConvoyManager unit tests (22 tests) |
| `internal/daemon/convoy_manager_integration_test.go` | ConvoyManager integration tests (2 tests, `//go:build integration`) |
| `internal/convoy/store_test.go` | Observer store helpers (3 tests) |
| `internal/convoy/operations_test.go` | Operations function edge cases + safety guard tests |
| `internal/daemon/daemon_test.go` | Daemon-level manager lifecycle (2 convoy tests) |

### Design Documents

| File | Contents |
|------|----------|
| `docs/design/convoy/convoy-lifecycle.md` | Problem statement, design principles, flow diagram |
| `docs/design/convoy/testing.md` | Test plan, failure modes, invariants, harness scorecard |
| `docs/design/convoy/spec.md` | This document |
| `docs/design/daemon/convoy-manager.md` | ConvoyManager architecture diagram (SDK polling + stranded scan) |

---

## 7. Review Findings -> Story Mapping

| Finding | Story |
|---------|-------|
| Stream-based convoy-manager doc was stale | S-15 |
| Testing doc had stale stream/backoff and duplicate gap entries | S-15 |
| Lifecycle observer/manual-close claims were stale | S-15 |
| Spec file-map command/test counts drifted | S-15 |
| DONE stories needed explicit corrective handling | S-16 |
| Refinery observer root-path ambiguity remains | S-17 (verified) |
| Refinery root-path fix required | S-18 |

---

## 8. Priority Order

| Priority | Story | Effort | Risk Mitigated |
|----------|-------|--------|----------------|
| **P0** | S-09: Subprocess context cancellation | Small | Shutdown hangs (DONE) |
| **P0** | S-08: Start() double-call guard | Trivial | Duplicate goroutines (DONE) |
| **P1** | S-11: Test gap P1 (high blast-radius) | Medium | Unknown prefix/rig, batch overflow (DONE) |
| **P1** | S-14: Test infrastructure | Medium | Maintainability, negative assertions (DONE) |
| **P1** | S-17: Refinery observer root-path verification | Small | Cross-rig convoy visibility correctness (DONE) |
| **P1** | S-18: Fix refinery convoy observer town-root path | Trivial | Refinery convoy observer silently disabled (DONE) |
| **P2** | S-10: Resolved gt path in observer | Small | PATH reliability in daemon (DONE) |
| **P2** | S-12: Test gap P2 (error paths) | Small | Untested error recovery (DONE) |
| **P2** | S-16: Corrective follow-up for DONE stories | Small | Historical drift in completed stories (DONE) |
| **P3** | S-13: Test gap P3 (lifecycle edges) | Small | Edge case coverage (DONE) |

---

## 9. Non-Goals (This Spec)

These are documented in convoy-lifecycle.md as future work but are **not** in
scope for this spec:

- Convoy owner/requester field and targeted notifications (P2 in lifecycle doc)
- Convoy timeout/SLA (`due_at` field, overdue surfacing) (P3 in lifecycle doc)
- Convoy reopen command (implicit via add, explicit command deferred)
- Test clock injection for ConvoyManager (P3 in testing.md -- useful but not blocking)
