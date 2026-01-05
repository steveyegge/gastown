# Test Coverage and Quality Review

**Reviewed by**: polecat/gus
**Date**: 2026-01-04
**Issue**: gt-a02fj.9

## Executive Summary

- **80 test files** covering **32 out of 42 packages** (76% package coverage)
- **631 test functions** with 192 subtests (30% use table-driven pattern)
- **10 packages** with **0 test coverage** (2,452 lines)
- **1 confirmed flaky test** candidate
- Test quality is generally good with moderate mocking

---

## Coverage Gap Inventory

### Packages Without Tests (Priority Order)

| Priority | Package | Lines | Risk | Notes |
|----------|---------|-------|------|-------|
| **P0** | `internal/lock` | 402 | **CRITICAL** | Multi-agent lock management. Bugs cause worker collisions. Already has `execCommand` mockable for testing. |
| **P1** | `internal/events` | 295 | HIGH | Event bus for audit trail. Mutex-protected writes. Core observability. |
| **P1** | `internal/boot` | 242 | HIGH | Boot watchdog lifecycle. Spawns tmux sessions. |
| **P1** | `internal/checkpoint` | 216 | HIGH | Session crash recovery. Critical for polecat continuity. |
| **P2** | `internal/tui/convoy` | 601 | MEDIUM | TUI component. Harder to test but user-facing. |
| **P2** | `internal/constants` | 221 | LOW | Mostly configuration constants. Low behavioral risk. |
| **P3** | `internal/style` | 331 | LOW | Output formatting. Visual only. |
| **P3** | `internal/claude` | 80 | LOW | Claude settings parsing. |
| **P3** | `internal/wisp` | 52 | LOW | Ephemeral molecule I/O. Small surface. |
| **P4** | `cmd/gt` | 12 | TRIVIAL | Main entry point. Minimal code. |

**Total untested lines**: 2,452

---

## Flaky Test Candidates

### Confirmed: `internal/feed/curator_test.go`

**Issue**: Uses `time.Sleep()` for synchronization (lines 59, 71, 119, 138)

```go
// Give curator time to start
time.Sleep(50 * time.Millisecond)
...
// Wait for processing
time.Sleep(300 * time.Millisecond)
```

**Risk**: Flaky under load, CI delays, or slow machines.

**Fix**: Replace with channel-based synchronization or polling with timeout:
```go
// Wait for condition with timeout
deadline := time.Now().Add(time.Second)
for time.Now().Before(deadline) {
    if conditionMet() {
        break
    }
    time.Sleep(10 * time.Millisecond)
}
```

---

## Test Quality Analysis

### Strengths

1. **Table-driven tests**: 30% of tests use `t.Run()` (192/631)
2. **Good isolation**: Only 2 package-level test variables
3. **Dedicated integration tests**: 15 files with explicit integration/e2e naming
4. **Error handling**: 316 uses of `if err != nil` in tests
5. **No random data**: No `rand.` usage in tests (deterministic)
6. **Environment safety**: Uses `t.Setenv()` for clean env var handling

### Areas for Improvement

1. **`testing.Short()`**: Only 1 usage. Long-running tests should check this.
2. **External dependencies**: 26 tests skip when `bd` or `tmux` unavailable - consider mocking more.
3. **time.Sleep usage**: Found in `curator_test.go` - should be eliminated.

---

## Test Smells (Minor)

| Smell | Location | Severity | Notes |
|-------|----------|----------|-------|
| Sleep-based sync | `feed/curator_test.go` | HIGH | See flaky section |
| External dep skips | Multiple files | LOW | Reasonable for integration tests |
| Skip-heavy file | `tmux/tmux_test.go` | LOW | Acceptable - tmux not always available |

---

## Priority List for New Tests

### Immediate (P0)

1. **`internal/lock`** - Critical path
   - Test `Acquire()` with stale lock cleanup
   - Test `Check()` with live/dead PIDs
   - Test `CleanStaleLocks()` with mock tmux sessions
   - Test `DetectCollisions()`
   - Test concurrent lock acquisition (race detection)

### High Priority (P1)

2. **`internal/events`**
   - Test `Log()` file creation and append
   - Test `write()` mutex behavior
   - Test payload helpers
   - Test graceful handling when not in workspace

3. **`internal/boot`**
   - Test `IsRunning()` with stale markers
   - Test `AcquireLock()` / `ReleaseLock()` cycle
   - Test `SaveStatus()` / `LoadStatus()` round-trip
   - Test degraded mode path

4. **`internal/checkpoint`**
   - Test `Read()` / `Write()` round-trip
   - Test `Capture()` git state extraction
   - Test `IsStale()` with various durations
   - Test `Summary()` output

### Medium Priority (P2)

5. **`internal/tui/convoy`** - Consider golden file tests for view output
6. **`internal/constants`** - Test any validation logic

---

## Missing Test Types

| Type | Current State | Recommendation |
|------|--------------|----------------|
| Unit tests | Good coverage where present | Add for P0-P1 packages |
| Integration tests | 15 dedicated files | Adequate |
| E2E tests | `browser_e2e_test.go` | Consider more CLI E2E |
| Fuzz tests | None | Consider for parsers (`formula/parser.go`) |
| Benchmark tests | None visible | Add for hot paths (`lock`, `events`) |

---

## Actionable Next Steps

1. **Fix flaky test**: Refactor `feed/curator_test.go` to use channels/polling
2. **Add lock tests**: Highest priority - bugs here break multi-agent
3. **Add events tests**: Core observability must be tested
4. **Add checkpoint tests**: Session recovery is critical path
5. **Run with race detector**: `go test -race ./...` to catch data races
6. **Consider `-short` flag**: Add `testing.Short()` checks to slow tests
