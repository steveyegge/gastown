# Test Analysis: Convoy Stage & Launch

Testing plan for `gt convoy stage` and `gt convoy launch` (PRD: `stage-launch/prd.md`).

---

## Critical Invariants

| # | Invariant | Category | Blast Radius | Currently Tested? |
|---|-----------|----------|-------------|-------------------|
| I-1 | Cycles in blocking deps MUST prevent convoy creation | data | **high** | no |
| I-2 | Beads with no valid rig MUST prevent convoy creation | data | **high** | no |
| I-3 | Errors MUST produce non-zero exit code and no side effects | safety | **high** | no |
| I-4 | Wave 1 contains ONLY tasks with zero unsatisfied blocking deps | data | **high** | no |
| I-5 | `parent-child` deps NEVER create execution edges | data | **high** | partial (feeder tests) |
| I-6 | Epics and non-slingable types are NEVER placed in waves | data | **high** | partial (`IsSlingableType`) |
| I-7 | Daemon MUST NOT feed issues from `staged:*` convoys | safety | **high** | no |
| I-8 | `--launch` on `staged_warnings` MUST require `--force` | safety | medium | no |
| I-9 | Re-staging a convoy MUST NOT create duplicates | data | medium | no |
| I-10 | `--json` output MUST be parseable JSON on stdout | data | medium | no |
| I-11 | Wave computation is deterministic for the same input DAG | data | medium | no |
| I-12 | All beads validated to exist before any convoy creation | safety | medium | no |
| I-13 | Launch dispatches ONLY Wave 1, not subsequent waves | safety | **high** | no |
| I-14 | Dispatch failure for one task does NOT abort remaining Wave 1 tasks | liveness | medium | no |
| I-15 | `gt convoy launch <id>` on already-open convoy MUST error | safety | low | no |

---

## Failure Modes

### Input failures

| # | Failure | Likelihood | Detection | Recovery | Covered? |
|---|---------|-----------|-----------|----------|----------|
| F-01 | Nonexistent bead ID passed | high | immediate (bd show fails) | user fixes typo | no |
| F-02 | Empty args (no bead IDs) | medium | immediate (arg validation) | usage help | no |
| F-03 | Mixed input types (epic + task IDs) | medium | ambiguous | detect and error | no |
| F-04 | Bead ID is actually a rig name | low | prefix check | error with hint | no |
| F-05 | Convoy ID passed to stage that is already `open` | medium | status check | re-analyze and warn | no |
| F-06 | Flag-like bead ID (`--verbose`) | low | immediate | reject | no |

### State failures

| # | Failure | Likelihood | Detection | Recovery | Covered? |
|---|---------|-----------|-----------|----------|----------|
| F-07 | Cycle in blocks deps (A blocks B blocks A) | medium | cycle detection | report path, refuse staging | no |
| F-08 | Orphan tasks (no parent, no deps) in epic tree | medium | reachability check | warn, still stage | no |
| F-09 | Stale convoy: re-stage finds beads deleted since first stage | low | bd show fails | error per-bead | no |
| F-10 | Concurrent staging of same beads by two users | low | no detection | last write wins (acceptable) | no |
| F-11 | Partial dep-add failure during convoy creation | medium | per-dep error check | partial tracking (existing pattern) | partial |
| F-12 | Epic has no children (empty DAG) | low | immediate | error: nothing to stage | no |

### Dependency failures

| # | Failure | Likelihood | Detection | Recovery | Covered? |
|---|---------|-----------|-----------|----------|----------|
| F-13 | `bd` binary not on PATH | low | exec error | clear error message | no |
| F-14 | `bd dep list` returns malformed JSON | low | json.Unmarshal fails | error with raw output | no |
| F-15 | `routes.jsonl` missing or corrupt | medium | parse error | error: cannot resolve rigs | partial |
| F-16 | `gt sling` fails during launch dispatch | medium | exec error | continue to next task, report | no |
| F-17 | Dolt store unavailable during re-stage | low | store open fails | error | no |

### Timing failures

| # | Failure | Likelihood | Detection | Recovery | Covered? |
|---|---------|-----------|-----------|----------|----------|
| F-18 | Bead closed between stage and launch | medium | status check at launch | skip closed, log | no |
| F-19 | Rig parked between stage and launch | medium | park check at launch | skip parked rig, warn | no |
| F-20 | New deps added between stage and launch | low | re-analyze at launch | waves may differ (acceptable) | no |

### Configuration failures

| # | Failure | Likelihood | Detection | Recovery | Covered? |
|---|---------|-----------|-----------|----------|----------|
| F-21 | Not inside a workspace (no mayor/rig/) | medium | workspace.FindFromCwd | clear error | no |
| F-22 | Bead prefix maps to town root (path=".") | low | rig resolve returns "" | error: no valid rig | partial |
| F-23 | Unknown bead prefix (not in routes.jsonl) | medium | GetRigNameForPrefix "" | error with suggestion | partial |

---

## Test Strategy Matrix

### Unit tests (package `cmd`, `convoy`)

These test pure logic in isolation using in-memory data. No Dolt, no bd stubs.

| ID | Test | Function Under Test | Failure Modes | Invariants | Priority |
|----|------|---------------------|---------------|------------|----------|
| U-01 | Cycle detection: simple A->B->A | `detectCycles(dag)` | F-07 | I-1 | P0 |
| U-02 | Cycle detection: no cycle (linear chain) | `detectCycles(dag)` | F-07 | I-1 | P0 |
| U-03 | Cycle detection: self-loop (A blocks A) | `detectCycles(dag)` | F-07 | I-1 | P0 |
| U-04 | Cycle detection: diamond (no cycle) | `detectCycles(dag)` | F-07 | I-1 | P0 |
| U-05 | Cycle detection: long chain with back-edge | `detectCycles(dag)` | F-07 | I-1 | P0 |
| U-06 | Wave computation: 3 independent tasks -> all Wave 1 | `computeWaves(dag)` | -- | I-4 | P0 |
| U-07 | Wave computation: linear chain A->B->C -> 3 waves | `computeWaves(dag)` | -- | I-4 | P0 |
| U-08 | Wave computation: diamond deps -> 3 waves | `computeWaves(dag)` | -- | I-4 | P0 |
| U-09 | Wave computation: mixed parallel + serial | `computeWaves(dag)` | -- | I-4 | P0 |
| U-10 | Wave computation: deterministic output (run 100x) | `computeWaves(dag)` | -- | I-11 | P1 |
| U-11 | Wave computation: excludes epics from waves | `computeWaves(dag)` | -- | I-6 | P0 |
| U-12 | Wave computation: excludes non-slingable types | `computeWaves(dag)` | -- | I-6 | P0 |
| U-13 | Wave computation: parent-child deps don't create edges | `computeWaves(dag)` | -- | I-5 | P0 |
| U-14 | Wave computation: empty DAG -> error | `computeWaves(dag)` | F-12 | -- | P1 |
| U-15 | DAG construction: blocks deps create edges | `buildDAG(beads)` | -- | I-4 | P0 |
| U-16 | DAG construction: conditional-blocks create edges | `buildDAG(beads)` | -- | I-4 | P1 |
| U-17 | DAG construction: waits-for creates edges | `buildDAG(beads)` | -- | I-4 | P1 |
| U-18 | DAG construction: parent-child recorded but no edge | `buildDAG(beads)` | -- | I-5 | P0 |
| U-19 | DAG construction: related/tracks deps ignored | `buildDAG(beads)` | -- | -- | P2 |
| U-20 | Error categorization: cycle is error (not warning) | `categorize(findings)` | -- | I-1, I-3 | P0 |
| U-21 | Error categorization: no-rig is error | `categorize(findings)` | -- | I-2, I-3 | P0 |
| U-22 | Error categorization: parked rig is warning | `categorize(findings)` | -- | -- | P1 |
| U-23 | Error categorization: orphan is warning | `categorize(findings)` | -- | -- | P1 |
| U-24 | Error categorization: missing integration branch is warning | `categorize(findings)` | -- | -- | P2 |
| U-25 | Status selection: no errors + no warnings -> staged_ready | `chooseStatus(errs, warns)` | -- | I-3 | P0 |
| U-26 | Status selection: warnings only -> staged_warnings | `chooseStatus(errs, warns)` | -- | -- | P0 |
| U-27 | Status selection: any error -> no creation | `chooseStatus(errs, warns)` | -- | I-3 | P0 |
| U-28 | Tree display: flat task list -> no tree, just list | `renderTree(dag)` | -- | -- | P2 |
| U-29 | Tree display: epic with nested sub-epics | `renderTree(dag)` | -- | -- | P2 |
| U-30 | Wave table display: includes blockers column | `renderWaveTable(waves)` | -- | -- | P2 |
| U-31 | JSON output: valid JSON, all required fields present | `renderJSON(result)` | -- | I-10 | P1 |
| U-32 | JSON output: errors array populated on failure | `renderJSON(result)` | -- | I-10 | P1 |
| U-33 | JSON output: convoy_id empty when errors found | `renderJSON(result)` | -- | I-3, I-10 | P1 |
| U-34 | Error categorization: cross-rig routing mismatch is warning | `categorize(findings)` | -- | -- | P1 |
| U-35 | Error categorization: capacity estimation is warning | `categorize(findings)` | -- | -- | P2 |
| U-36 | Tree node rendering: includes bead ID, title, type, status, rig | `renderTreeNode(node)` | -- | -- | P1 |
| U-37 | Tree node rendering: blocked tasks show blockers inline | `renderTreeNode(node)` | -- | -- | P1 |
| U-38 | Wave table summary line: total waves, tasks, parallelism per wave | `renderWaveTable(waves)` | -- | -- | P1 |
| U-39 | Error output: each error includes suggested fix text | `renderErrors(errs)` | -- | I-3 | P1 |

### Integration tests (bd stub + workspace, package `cmd`)

These test the full command flow with stubbed `bd` and `gt` binaries.

| ID | Test | User Story | Failure Modes | Invariants | Priority |
|----|------|-----------|---------------|------------|----------|
| IT-01 | Stage epic: walks parent-child tree, collects all descendants | US-001 | F-01 | I-12 | P0 |
| IT-02 | Stage epic: nonexistent child fails entire stage | US-001, US-002 | F-01, F-09 | I-3, I-12 | P0 |
| IT-03 | Stage task list: analyzes only given tasks | US-001 | -- | -- | P0 |
| IT-04 | Stage convoy: reads tracked beads from existing convoy | US-001 | F-05 | -- | P1 |
| IT-05 | Stage with cycle: refuses to create convoy | US-002 | F-07 | I-1, I-3 | P0 |
| IT-06 | Stage with no valid rig: refuses to create convoy | US-002 | F-23 | I-2, I-3 | P0 |
| IT-07 | Stage with errors: exit code non-zero, no bd create called | US-002 | -- | I-3 | P0 |
| IT-08 | Stage with parked rig: creates convoy as staged_warnings | US-003 | F-19 | -- | P1 |
| IT-09 | Stage epic with unreachable task (not in descendant tree): warns, creates staged_warnings | US-003 | F-08 | -- | P1 |
| IT-10 | Stage clean: creates convoy as staged_ready | US-007 | -- | -- | P0 |
| IT-11 | Stage convoy: tracks all slingable beads via deps | US-007 | F-11 | -- | P0 |
| IT-12 | Stage convoy: description includes wave count + timestamp | US-007 | -- | -- | P2 |
| IT-13 | Re-stage: updates status, no duplicate convoy | US-007 | F-09 | I-9 | P1 |
| IT-14 | Launch staged_ready: transitions to open, dispatches Wave 1 | US-008 | F-16 | I-13 | P0 |
| IT-15 | Launch staged_warnings without --force: errors | US-008 | -- | I-8 | P0 |
| IT-16 | Launch staged_warnings with --force: dispatches | US-008 | -- | I-8 | P1 |
| IT-17 | Launch dispatch failure: continues to next task | US-008 | F-16 | I-14 | P1 |
| IT-18 | Launch already-open convoy: errors | US-010 | F-05 | I-15 | P1 |
| IT-19 | `gt convoy launch <epic>` = `gt convoy stage <epic> --launch` | US-010 | -- | -- | P1 |
| IT-20 | `gt convoy launch <task1> <task2>` works as alias | US-010 | -- | -- | P1 |
| IT-21 | --json output: valid JSON with all fields | US-011 | -- | I-10 | P1 |
| IT-22 | --json output: no human-readable text on stdout | US-011 | -- | I-10 | P1 |
| IT-23 | Stage with --launch: full end-to-end (stage + dispatch) | US-008 | -- | I-4, I-13 | P0 |
| IT-24 | Empty args: usage error | -- | F-02 | -- | P2 |
| IT-25 | Flag-like bead ID: rejected | -- | F-06 | -- | P2 |
| IT-26 | Stage with missing integration branch: warns, creates staged_warnings | US-003 | -- | -- | P1 |
| IT-27 | Stage with cross-rig routing mismatch: warns, includes in output | US-003 | -- | -- | P1 |
| IT-28 | Stage with capacity warning: informational, creates staged_warnings | US-003 | -- | -- | P2 |
| IT-29 | Launch output: convoy ID + `gt convoy status` command printed | US-009 | -- | -- | P1 |
| IT-30 | Launch output: each dispatched task shows polecat name | US-009 | -- | -- | P1 |
| IT-31 | Launch output: TUI hint (`gt convoy -i`) printed | US-009 | -- | -- | P2 |
| IT-32 | Launch output: daemon feed explanation printed | US-009 | -- | -- | P2 |
| IT-33 | Launch staged_ready convoy: skips re-analysis, dispatches directly | US-010 | -- | -- | P0 |
| IT-34 | --json with errors: non-zero exit code | US-011 | -- | I-3, I-10 | P1 |
| IT-35 | Mixed input (epic + task IDs): errors with hint | -- | F-03 | -- | P1 |
| IT-36 | Bead ID that looks like rig name: errors with hint | -- | F-04 | -- | P2 |
| IT-37 | bd binary not on PATH: clear error message | -- | F-13 | -- | P2 |
| IT-38 | Malformed JSON from bd: error with raw output | -- | F-14 | -- | P2 |
| IT-39 | Not inside workspace: clear error | -- | F-21 | -- | P2 |
| IT-40 | Display ordering: tree printed before wave table | US-005, US-006 | -- | -- | P2 |
| IT-41 | Stage clean: convoy ID printed to stdout | US-007 | -- | -- | P1 |
| IT-42 | Bead closed between stage and launch: skip closed, log | US-008 | F-18 | -- | P1 |
| IT-43 | Stage epic with isolated task (no blocking deps from other staged tasks): warns (epic input only — task-list input never warns for isolation) | US-003 | F-08 | -- | P1 |
| IT-44 | Stage with missing/corrupt routes.jsonl: errors with clear message | US-002 | F-15 | I-2 | P2 |

### Integration tests (real Dolt store, package `convoy`)

These test DAG walking and wave computation against a real beads store.

| ID | Test | User Story | Failure Modes | Invariants | Priority |
|----|------|-----------|---------------|------------|----------|
| DS-01 | Epic tree walk: collects all descendants (3 levels deep) | US-001 | -- | -- | P0 |
| DS-02 | Epic tree walk: handles cross-rig external references | US-001 | -- | -- | P1 |
| DS-03 | Wave computation with real deps: 3-wave linear chain | US-004 | -- | I-4, I-5 | P0 |
| DS-04 | Wave computation with real deps: parallel + serial mixed | US-004 | -- | I-4 | P0 |
| DS-05 | Cycle detection with real store: 2-node cycle | US-002 | F-07 | I-1 | P0 |
| DS-06 | isIssueBlocked integration: blocked task not in Wave 1 | US-004 | -- | I-4 | P1 |
| DS-07 | Event-driven path skips staged_ready convoy (`CheckConvoysForIssue` → `feedNextReadyIssue`) | US-007 | -- | I-7 | P0 |
| DS-08 | Event-driven path skips staged_warnings convoy | US-007 | -- | I-7 | P0 |
| DS-09 | Stranded scan path excludes staged convoys (`findStrandedConvoys` queries `--status=open`) | US-007 | -- | I-7 | P0 |
| DS-10 | Daemon feeds convoy after status transitions from staged_ready to open | US-008 | -- | I-7 | P1 |

### Snapshot tests (package `cmd`)

Capture and verify console output format stability.

| ID | Test | User Story | Priority |
|----|------|-----------|----------|
| SN-01 | Tree display: epic with 2 sub-epics, 5 tasks | US-005 | P2 |
| SN-02 | Wave table: 3 waves with blockers column | US-006 | P2 |
| SN-03 | Launch output: convoy ID, wave summary, polecat list, hints | US-009 | P2 |
| SN-04 | Error output: cycle path formatting | US-002 | P2 |
| SN-05 | Warning output: parked rig + orphan list | US-003 | P2 |
| SN-06 | JSON output: full structure | US-011 | P2 |

### Property tests (package `cmd` or `convoy`)

Prove invariants hold over randomized DAGs.

| ID | Test | Invariant | Priority |
|----|------|-----------|----------|
| PT-01 | Random acyclic DAG: wave computation terminates, Wave 1 non-empty | I-4 | P1 |
| PT-02 | Random acyclic DAG: every task appears in exactly one wave | I-4 | P1 |
| PT-03 | Random acyclic DAG: no task appears before its blocker's wave | I-4 | P1 |
| PT-04 | Random DAG with cycle: cycle detection always finds it | I-1 | P1 |
| PT-05 | Random acyclic DAG: wave assignment is deterministic (same input -> same output) | I-11 | P2 |
| PT-06 | Random acyclic DAG: parent-child edges never affect wave assignment | I-5 | P1 |

---

## Harness Scorecard

Assessment of existing test infrastructure for stage/launch needs.

| Dimension | Score (1-5) | Key Gap |
|-----------|-------------|---------|
| Fixtures & Setup | **3** | `setupTownWithBdStub` and `setupTestStore` exist but no DAG fixture builder; each test hand-rolls bead graphs |
| Isolation | **4** | Tests use temp dirs, PATH injection, chdir with cleanup; parallel-safe except for `os.Chdir` (serializes cmd tests) |
| Observability | **3** | bd stub logs capture commands; but no structured assertion on DAG shape or wave output |
| Speed | **4** | Unit tests <1s, Dolt store tests ~0.1s each, full cmd suite ~10s; acceptable |
| Determinism | **4** | No known flaky tests; Dolt store tests skip when unavailable rather than flake |

### Key gaps for stage/launch

1. **No DAG fixture builder.** Every test that needs a bead graph with deps will hand-roll `bd show`/`dep list` JSON responses. A shared helper that declares a graph structure and generates the bd stub script would eliminate duplication and make tests readable.

2. **No wave assertion helpers.** Tests will need to verify "task X is in Wave N" and "Wave N has exactly K tasks." Without a helper, every test parses raw output or maintains parallel data structures.

3. **`os.Chdir` serialization.** The `setupTownWithBdStub` pattern uses `os.Chdir` which is process-global. This prevents `t.Parallel()` on any test that needs workspace resolution. Stage/launch tests will inherit this limitation.

4. **No snapshot infrastructure.** Display tests (tree, wave table, launch output) need golden-file comparison. Currently no snapshot tooling exists in the repo.

---

## Tooling Recommendations

### DAG Fixture Builder

**Problem:** Every integration test that involves beads with deps must manually construct bd stub scripts with hardcoded JSON. This is error-prone, verbose (~30 lines per test), and makes the graph structure invisible.

**Proposal:** A `dagBuilder` test helper that accepts a declarative graph definition and produces:
- A bd stub script that returns correct JSON for `show`, `dep list`, and `list` commands
- A routes.jsonl file for rig resolution
- Optionally, real Dolt store issues + deps (for DS-* tests)

```go
dag := newTestDAG(t).
    Epic("epic-1", "Root Epic").
    Task("task-1", "First task", rig("gastown")).ParentOf("epic-1").
    Task("task-2", "Second task", rig("gastown")).ParentOf("epic-1").BlockedBy("task-1").
    Task("task-3", "Third task", rig("gastown")).ParentOf("epic-1").BlockedBy("task-2")
// dag.BdStubScript() -> shell script
// dag.RoutesJSONL() -> routes file content
// dag.Populate(store) -> creates issues + deps in real Dolt store
```

**Compound Value:** Every new convoy test (stage, launch, future milestones) reuses this. The graph definition IS the test documentation.

**Exists Today?** No. `molecule_dag.go` has `buildDAG` but it operates on live beads, not test fixtures.

**Priority:** P0 -- build this before writing integration tests.

### Wave Assertion Helper

**Problem:** Verifying wave assignments requires either parsing console output or maintaining a parallel `map[string]int` and comparing element-by-element. Both are fragile.

**Proposal:** A `waveAssert` helper:

```go
waveAssert(t, waves).
    Wave(1, "task-1", "task-3").     // unordered within wave
    Wave(2, "task-2").
    Wave(3, "task-4", "task-5").
    NoTask("epic-1").                // epics excluded
    Total(3)                         // 3 waves
```

**Compound Value:** Used by every wave computation test (U-06 through U-14, IT-14, IT-23, DS-03, DS-04, PT-01 through PT-06).

**Exists Today?** No.

**Priority:** P1 -- build alongside wave computation implementation.

### Property Test Harness

**Problem:** Wave computation correctness is hard to exhaustively verify with hand-crafted cases. Random DAGs expose edge cases (wide graphs, deep chains, disconnected components) that humans miss.

**Proposal:** Use `testing/quick` or a custom random DAG generator:

```go
func randomAcyclicDAG(seed int64, nodes, edges int) *testDAG { ... }
func randomDAGWithCycle(seed int64, nodes int) *testDAG { ... }
```

Combined with property assertions: "every task in exactly one wave", "no task before its blocker", "cycle always detected."

**Compound Value:** Catches regressions from any future DAG algorithm changes. Seed is logged on failure for reproduction.

**Exists Today?** No random graph generators exist. `testing/quick` is available in stdlib.

**Priority:** P1 -- write after core wave computation is stable.

---

## Test Priority Summary

### P0 — Must ship with the feature (38 tests)

| Tier | Count | Tests |
|------|-------|-------|
| Unit | 19 | U-01..U-09, U-11, U-12, U-13, U-15, U-18, U-20, U-21, U-25, U-26, U-27 |
| Integration (bd stub) | 12 | IT-01, IT-02, IT-03, IT-05, IT-06, IT-07, IT-10, IT-11, IT-14, IT-15, IT-23, IT-33 |
| Integration (Dolt) | 7 | DS-01, DS-03, DS-04, DS-05, DS-07, DS-08, DS-09 |

### P1 — Ship within 1 week of feature (42 tests)

| Tier | Count | Tests |
|------|-------|-------|
| Unit | 14 | U-10, U-14, U-16, U-17, U-22, U-23, U-31, U-32, U-33, U-34, U-36, U-37, U-38, U-39 |
| Integration (bd stub) | 20 | IT-04, IT-08, IT-09, IT-13, IT-16, IT-17, IT-18, IT-19, IT-20, IT-21, IT-22, IT-26, IT-27, IT-29, IT-30, IT-34, IT-35, IT-41, IT-42, IT-43 |
| Integration (Dolt) | 3 | DS-02, DS-06, DS-10 |
| Property | 5 | PT-01, PT-02, PT-03, PT-04, PT-06 |

### P2 — Nice to have (25 tests)

| Tier | Count | Tests |
|------|-------|-------|
| Unit | 6 | U-19, U-24, U-28, U-29, U-30, U-35 |
| Integration (bd stub) | 12 | IT-12, IT-24, IT-25, IT-28, IT-31, IT-32, IT-36, IT-37, IT-38, IT-39, IT-40, IT-44 |
| Snapshot | 6 | SN-01 through SN-06 |
| Property | 1 | PT-05 |

**Total: 105 tests (38 P0 + 42 P1 + 25 P2)**

---

## Anti-Patterns to Watch For

| Pattern | Risk in Stage/Launch | Mitigation |
|---------|---------------------|------------|
| Sleep-based sync | Daemon integration tests (DS-09, DS-10) wait for convoy status transitions; temptation to add delays | Use completion channels or check bd state, never `time.Sleep` in tests |
| God fixtures | One massive DAG used by all tests | Each test declares its own DAG via the builder; share nothing |
| Assertion-free tests | "It didn't panic" is not a test | Every test asserts on: wave assignment, error/warning categorization, or bd commands logged |
| Snapshot overload | Tempting to snapshot all console output | Use snapshots only for display format (SN-*); use specific assertions for logic (waves, errors) |
| Test-after-the-fact | Writing tests to hit coverage after impl | Write U-01 through U-13 (cycle + wave) BEFORE implementing; they define the contract |
| Environment coupling | Tests relying on real `bd` or `gt` on PATH | Always use stub binaries in bin/ with PATH override; never depend on system binaries |

---

## Next Actions

1. **Build `dagBuilder` test helper** (P0 tooling) -- shared fixture builder for declaring bead graphs. Output: bd stub script, routes.jsonl, optional Dolt population.

2. **Write cycle detection + wave computation unit tests** (U-01 through U-13) -- these define the algorithm contract before implementation begins. TDD.

3. **Implement cycle detection + wave computation** -- `detectCycles()`, `computeWaves()` as pure functions on in-memory DAG. No I/O.

4. **Write integration tests for staging flow** (IT-01 through IT-07, IT-10, IT-11) -- full command flow with bd stubs.

5. **Implement `gt convoy stage`** -- bead validation, DAG construction, analysis, convoy creation.

6. **Write daemon integration tests** (DS-07, DS-08, DS-09) -- staged convoys must not be fed via either feeding path (event-driven + stranded scan).

7. **Implement daemon staged-convoy guard** -- add staged-status check in `CheckConvoysForIssue` (event-driven path, `operations.go`). Stranded scan path (`convoy.go:1231`) is already safe via `--status=open` query. Update `ensureKnownConvoyStatus` and `validateConvoyStatusTransition` for staged statuses.

8. **Write launch tests** (IT-14, IT-15, IT-23) -- dispatch Wave 1 only.

9. **Implement `gt convoy launch`** -- status transition, Wave 1 dispatch.

10. **Build `waveAssert` helper + property tests** (PT-01 through PT-04) -- prove invariants over random graphs.

---

## PRD Cross-Reference

Every acceptance criterion in the PRD mapped to its covering test(s).

### US-001: Bead validation and DAG construction

| AC | Criterion | Tests |
|---|---|---|
| 1 | bd show each bead, error if missing | IT-01, IT-02 |
| 2 | Epic: walks full parent-child tree recursively | IT-01, DS-01, DS-02 |
| 3 | Task list: analyzes only given tasks | IT-03 |
| 4 | Convoy: reads tracked beads via dep list | IT-04 |
| 5 | DAG from blocks/conditional-blocks/waits-for | U-15, U-16, U-17 |
| 6 | parent-child: recorded, no execution edges | U-13, U-18, PT-06 |

### US-002: Error detection

| AC | Criterion | Tests |
|---|---|---|
| 1 | Cycles detected with cycle path | U-01..U-05, IT-05, DS-05, SN-04, PT-04 |
| 2 | No valid rig detected | IT-06, U-21 |
| 3 | Errors: no convoy, no status changes | IT-07, U-27 |
| 4 | Error output: bead IDs + suggested fix | U-39, SN-04 |
| 5 | Non-zero exit code on errors | IT-07 |

### US-003: Warning detection

| AC | Criterion | Tests |
|---|---|---|
| 1 | Orphan detection (epic input only: unreachable from descendant tree + isolated in wave graph) | IT-09, IT-43, U-23 |
| 2 | Missing integration branches | U-24, IT-26 |
| 3 | Parked rigs | IT-08, U-22 |
| 4 | Cross-rig routing warnings | U-34, IT-27 |
| 5 | Capacity estimation | U-35, IT-28 |
| 6 | Warnings distinguished from errors | U-20..U-24 |
| 7 | Warnings only -> staged_warnings | IT-08, U-26 |

### US-004: Wave computation

| AC | Criterion | Tests |
|---|---|---|
| 1 | Wave 1 = no unsatisfied blocking deps | U-06..U-09, DS-03, PT-01..PT-03 |
| 2 | Wave N+1 = blockers all in earlier waves | U-07, U-08, PT-03 |
| 3 | No blocking deps = Wave 1 | U-06, PT-01 |
| 4 | Epics/non-slingable excluded | U-11, U-12 |
| 5 | Full descendant set or just given tasks | IT-01 + IT-03 (input), U-06..U-09 (waves) |

### US-005: DAG tree display

| AC | Criterion | Tests |
|---|---|---|
| 1 | Epic: full tree with indentation | U-29, SN-01 |
| 2 | Each node: bead ID, title, type, status, rig | U-36, SN-01 |
| 3 | Sub-epics visually distinct | SN-01 |
| 4 | Blocked tasks show blockers inline | U-37, SN-01 |
| 5 | Task list: flat list | U-28 |
| 6 | Tree displayed before wave table | IT-40 |

### US-006: Wave dispatch plan display

| AC | Criterion | Tests |
|---|---|---|
| 1 | Table: wave #, IDs, titles, rig, blockers | U-30, SN-02 |
| 2 | Displayed after DAG tree | IT-40 |
| 3 | Summary line: waves, tasks, parallelism | U-38, SN-02 |
| 4 | Warnings after wave table for staged_warnings | SN-05 |

### US-007: Convoy creation with staged status

| AC | Criterion | Tests |
|---|---|---|
| 1 | No errors, no warnings -> staged_ready | IT-10, U-25 |
| 2 | Warnings only -> staged_warnings | IT-08, U-26 |
| 3 | Errors -> no convoy | IT-07, U-27 |
| 4 | Tracks all slingable beads | IT-11 |
| 5 | Description: wave count, task count, timestamp | IT-12 |
| 6 | Convoy ID printed to console | IT-41 |
| 7 | Re-staging updates status, no duplicate | IT-13 |

### US-008: Launch — dispatch Wave 1

| AC | Criterion | Tests |
|---|---|---|
| 1 | Transitions staged_ready -> open | IT-14 |
| 2 | staged_warnings requires --force | IT-15, IT-16 |
| 3 | Dispatches Wave 1 via internal Go dispatch | IT-14, IT-23 |
| 4 | Subsequent waves NOT dispatched | IT-14, IT-23 |
| 5 | Dispatch failure continues | IT-17 |

### US-009: Launch console output

| AC | Criterion | Tests |
|---|---|---|
| 1 | Convoy ID + status command | IT-29, SN-03 |
| 2 | Wave summary | SN-03 |
| 3 | Each Wave 1 task with polecat | IT-30, SN-03 |
| 4 | TUI hint (gt convoy -i) | IT-31, SN-03 |
| 5 | Daemon feeds explanation | IT-32, SN-03 |

### US-010: gt convoy launch as alias

| AC | Criterion | Tests |
|---|---|---|
| 1 | launch epic = stage epic --launch | IT-19 |
| 2 | launch task1 task2 works | IT-20 |
| 3 | launch staged_ready: no re-analysis | IT-33 |
| 4 | launch staged_warnings requires --force | IT-15, IT-16 |
| 5 | launch already-open errors | IT-18 |

### US-011: JSON output

| AC | Criterion | Tests |
|---|---|---|
| 1 | --json outputs JSON to stdout | IT-21, U-31 |
| 2 | All required fields present | U-31, U-32, U-33, SN-06 |
| 3 | Human-readable suppressed | IT-22 |
| 4 | Non-zero exit code on errors (JSON mode) | IT-34 |

### Failure modes coverage

| FM | Failure | Tests |
|---|---|---|
| F-01 | Nonexistent bead ID | IT-01, IT-02 |
| F-02 | Empty args | IT-24 |
| F-03 | Mixed input types | IT-35 |
| F-04 | Bead ID looks like rig name | IT-36 |
| F-05 | Convoy already open | IT-18 |
| F-06 | Flag-like bead ID | IT-25 |
| F-07 | Cycle in blocks deps | U-01..U-05, IT-05, DS-05, PT-04 |
| F-08 | Orphan tasks | IT-09, IT-43 |
| F-09 | Stale convoy on re-stage | IT-02, IT-13 |
| F-10 | Concurrent staging | _(accepted risk, no test)_ |
| F-11 | Partial dep-add failure | IT-11 (existing pattern) |
| F-12 | Epic has no children | U-14 |
| F-13 | bd not on PATH | IT-37 |
| F-14 | Malformed JSON from bd | IT-38 |
| F-15 | routes.jsonl missing | IT-06, IT-44 |
| F-16 | gt sling fails during launch | IT-17 |
| F-17 | Dolt unavailable | _(skips via setupTestStore)_ |
| F-18 | Bead closed between stage and launch | IT-42 |
| F-19 | Rig parked between stage and launch | IT-08 |
| F-20 | New deps between stage and launch | _(accepted risk, re-analysis handles)_ |
| F-21 | Not inside workspace | IT-39 |
| F-22 | Bead prefix maps to town root | IT-06 (existing coverage) |
| F-23 | Unknown bead prefix | IT-06 |
