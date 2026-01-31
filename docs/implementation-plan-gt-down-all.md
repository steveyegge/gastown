# Implementation Plan: `gt down --all` (v2.4)

> Based on [proposal-gt-stop-all-v2.md](./proposal-gt-stop-all-v2.md) v2.4

## Overview

**Goal**: Implement comprehensive shutdown for Gas Town in ~250 lines across 6 sequential feature branches.

**Branch Strategy**: Each branch builds on the previous, cherry-picked from `main`.

```
main
 └─> feat/gt-down-refinery      (+30 lines)   - Bug fix: add refinery shutdown
      └─> feat/gt-down-bd-processes  (+80 lines)  - Add bd process management
           └─> feat/gt-down-flags    (+20 lines)  - New CLI flags
                └─> feat/gt-down-lock (+30 lines) - Shutdown lock
                     └─> feat/gt-down-verification (+40 lines) - Respawn detection
                          └─> feat/gt-down-tests  (+50 lines)  - Tests
```

---

## Branch 1: `feat/gt-down-refinery`

**Problem Solved**: P3 - Refineries keep running after `gt down`

**Files Changed**:
- `internal/cmd/down.go` (+30 lines)

**Changes**:
```go
// After line 74, add refinery loop BEFORE witnesses:

// 1a. Stop refineries first (NEW - was missing!)
for _, rigName := range rigs {
    sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
    if err := stopSession(t, sessionName); err != nil {
        printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
        allOK = false
    } else {
        printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "stopped")
    }
}

// 1b. Stop witnesses (existing code - renumber from 1 to 1b)
```

**Verification**:
```bash
go build ./...
go test ./internal/cmd/...
# Manual: Start a rig, run `gt down`, verify refinery session is killed
```

**Estimated Lines**: +30

---

## Branch 2: `feat/gt-down-bd-processes`

**Problem Solved**: P1 (bd daemon respawn), P2 (bd activity wakeups)

**Files Changed**:
- `internal/beads/daemon.go` (+80 lines)

**New Functions**:
```go
// StopAllBdProcesses stops all bd daemon and activity processes.
// Returns (daemonsKilled, activityKilled, error).
func StopAllBdProcesses(dryRun, force bool) (int, int, error)

// CountBdDaemons returns count of running bd daemons.
func CountBdDaemons() int

// CountBdActivityProcesses returns count of running bd activity processes.
func CountBdActivityProcesses() int

// Helper: parseBdDaemonCount parses bd daemon list --json output.
func parseBdDaemonCount(output []byte) int

// Helper: stopBdDaemons uses bd daemon killall.
func stopBdDaemons() int

// Helper: stopBdActivityProcesses with SIGTERM->wait->SIGKILL.
func stopBdActivityProcesses(force bool) int
```

**Verification**:
```bash
go build ./...
go test ./internal/beads/...
# Manual: Start bd daemon, call StopAllBdProcesses, verify stopped
```

**Estimated Lines**: +80

---

## Branch 3: `feat/gt-down-flags`

**Problem Solved**: P4 - Old `--all` too destructive (rename to `--nuke`)

**Files Changed**:
- `internal/cmd/down.go` (+20 lines)

**Changes**:

1. Add new variables:
```go
var (
    downQuiet   bool
    downForce   bool
    downAll     bool  // NEW MEANING: stop bd processes + verify
    downNuke    bool  // NEW: old --all behavior (kill tmux server)
    downDryRun  bool  // NEW: preview mode
)
```

2. Update `init()`:
```go
func init() {
    downCmd.Flags().BoolVarP(&downQuiet, "quiet", "q", false, "Only show errors")
    downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "Force kill without graceful shutdown")
    downCmd.Flags().BoolVarP(&downAll, "all", "a", false, "Stop bd daemons/activity and verify shutdown")
    downCmd.Flags().BoolVar(&downNuke, "nuke", false, "Kill entire tmux server (DESTRUCTIVE)")
    downCmd.Flags().BoolVar(&downDryRun, "dry-run", false, "Preview what would be stopped")
    rootCmd.AddCommand(downCmd)
}
```

3. Update Phase 7 (was `downAll`, now `downNuke`):
```go
// 4. Kill tmux server if --nuke (renamed from --all)
if downNuke {
    // Add GT_NUKE_ACKNOWLEDGED warning
    if os.Getenv("GT_NUKE_ACKNOWLEDGED") == "" {
        fmt.Printf("⚠ WARNING: --nuke kills ALL tmux sessions, not just Gas Town.\n")
        fmt.Printf("Set GT_NUKE_ACKNOWLEDGED=1 to suppress this warning.\n")
    }
    if err := t.KillServer(); err != nil {
        // ...
    }
}
```

**Verification**:
```bash
go build ./...
gt down --help  # Verify new flags appear
gt down --dry-run  # Verify dry-run works
```

**Estimated Lines**: +20

---

## Branch 4: `feat/gt-down-lock`

**Problem Solved**: P6 - Concurrent shutdowns race

**Files Changed**:
- `internal/cmd/down.go` (+30 lines)

**Changes**:

1. Add imports and constants:
```go
import (
    "context"
    "github.com/gofrs/flock"
)

const (
    shutdownLockFile    = "daemon/shutdown.lock"
    shutdownLockTimeout = 5 * time.Second
)
```

2. Add lock function:
```go
func acquireShutdownLock(townRoot string) (*flock.Flock, error) {
    lockPath := filepath.Join(townRoot, shutdownLockFile)
    if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
        return nil, fmt.Errorf("creating lock directory: %w", err)
    }
    lock := flock.New(lockPath)
    ctx, cancel := context.WithTimeout(context.Background(), shutdownLockTimeout)
    defer cancel()
    locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
    if err != nil {
        return nil, fmt.Errorf("lock acquisition failed: %w", err)
    }
    if !locked {
        return nil, fmt.Errorf("another shutdown is in progress")
    }
    return lock, nil
}
```

3. Use in `runDown()`:
```go
func runDown(cmd *cobra.Command, args []string) error {
    // ... existing workspace check ...

    // Phase 0: Acquire lock
    if !downDryRun {
        lock, err := acquireShutdownLock(townRoot)
        if err != nil {
            return err
        }
        defer lock.Unlock()
    }

    // ... rest of function ...
}
```

**Verification**:
```bash
go build ./...
# Manual: Run two `gt down` concurrently, second should fail with lock message
```

**Estimated Lines**: +30

---

## Branch 5: `feat/gt-down-verification`

**Problem Solved**: P5 - No verification of shutdown

**Files Changed**:
- `internal/cmd/down.go` (+40 lines)
- `internal/beads/daemon.go` (export helpers)

**Changes**:

1. Export count functions in `beads/daemon.go`:
```go
// Rename: countBdDaemons -> CountBdDaemons (export)
// Rename: countBdActivityProcesses -> CountBdActivityProcesses (export)
```

2. Add verification function in `down.go`:
```go
func verifyShutdown(t *tmux.Tmux, townRoot string) []string {
    var respawned []string
    
    // Check bd daemons
    if count := beads.CountBdDaemons(); count > 0 {
        respawned = append(respawned, fmt.Sprintf("bd daemon (%d running)", count))
    }
    
    // Check bd activity
    if count := beads.CountBdActivityProcesses(); count > 0 {
        respawned = append(respawned, fmt.Sprintf("bd activity (%d running)", count))
    }
    
    // Check gt-*/hq-* sessions
    sessions, _ := t.ListSessions()
    for _, sess := range sessions {
        if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
            respawned = append(respawned, fmt.Sprintf("session %s", sess))
        }
    }
    
    // Check daemon PID
    // ... (see proposal for full implementation)
    
    return respawned
}

func isProcessRunning(pid int) bool {
    return syscall.Kill(pid, 0) == nil
}
```

3. Add Phase 5 to `runDown()`:
```go
// Phase 5: Verification (--all only)
if downAll && !downDryRun {
    time.Sleep(500 * time.Millisecond)
    respawned := verifyShutdown(t, townRoot)
    if len(respawned) > 0 {
        fmt.Printf("⚠ Warning: Some processes may have respawned:\n")
        for _, r := range respawned {
            fmt.Printf("  • %s\n", r)
        }
        allOK = false
    }
}
```

4. Add Phase 1 (bd process stopping) in `runDown()`:
```go
// Phase 1: Stop Resurrection Layer (--all only)
if downAll {
    daemonsKilled, activityKilled, err := beads.StopAllBdProcesses(downDryRun, downForce)
    // ... report results ...
}
```

**Verification**:
```bash
go build ./...
gt down --all --dry-run  # Should show what would be stopped
gt down --all            # Should verify shutdown after stopping
```

**Estimated Lines**: +40

---

## Branch 6: `feat/gt-down-tests`

**Files Changed**:
- `internal/beads/daemon_test.go` (+30 lines)
- `internal/cmd/down_test.go` (+20 lines)

**Tests**:

```go
// internal/beads/daemon_test.go
func TestParseBdDaemonCount_Array(t *testing.T)
func TestParseBdDaemonCount_ObjectWithCount(t *testing.T)
func TestParseBdDaemonCount_Empty(t *testing.T)
func TestCountBdActivityProcesses(t *testing.T)  // Integration

// internal/cmd/down_test.go
func TestIsProcessRunning_CurrentProcess(t *testing.T)
func TestIsProcessRunning_InvalidPID(t *testing.T)
```

**Verification**:
```bash
go test ./internal/beads/... ./internal/cmd/... -v
```

**Estimated Lines**: +50

---

## Summary

| Branch | Problem(s) | Files | Lines | Depends On |
|--------|------------|-------|-------|------------|
| feat/gt-down-refinery | P3 | down.go | +30 | main |
| feat/gt-down-bd-processes | P1, P2 | daemon.go | +80 | Branch 1 |
| feat/gt-down-flags | P4 | down.go | +20 | Branch 2 |
| feat/gt-down-lock | P6 | down.go | +30 | Branch 3 |
| feat/gt-down-verification | P5 | down.go, daemon.go | +40 | Branch 4 |
| feat/gt-down-tests | - | *_test.go | +50 | Branch 5 |

**Total**: ~250 lines of new code

---

## Merge Strategy

```bash
# Option A: Squash merge each branch to main
git checkout main && git merge --squash feat/gt-down-refinery
git checkout main && git merge --squash feat/gt-down-bd-processes
# ... etc

# Option B: Create single PR from final branch
# feat/gt-down-tests contains all changes, PR that to main
```

---

## Rollback

If issues arise, the shutdown lock file can be manually removed:
```bash
rm ~/gt/daemon/shutdown.lock
```

Each branch is independently revertable since they're additive.
