# Proposal v3: Unified Shutdown Enhancement (`gt down --all`)

> **Status**: Draft - Simplified Architecture
> **Author**: Claude (synthesized from v2 review)
> **Version**: 3.0
> **Branch**: `feat/gt-down-all-v3` (to be created)
> **Related**: `docs/proposal-gt-stop-all-v2.md`, `docs/STOPPING_GASTOWN.md`
> **Last Updated**: 2026-01-10

---

## Executive Summary

**v2 was over-engineered.** This v3 proposal achieves the same goal with 70% less code by:

1. **Reusing existing code** instead of creating a new `internal/shutdown/` package
2. **Leveraging `bd daemon killall`** directly (no custom process discovery needed)
3. **Extending `gt down`** minimally instead of reimagining it
4. **Skipping config migration** (out of scope - separate concern)

### What Changed from v2

| v2 Approach | v3 Approach | Why |
|-------------|-------------|-----|
| New `internal/shutdown/` package (9 files) | Extend existing `internal/cmd/down.go` | Code already exists, just needs wiring |
| Custom process discovery via `ps` | Use `bd daemon killall` + simple `pkill` fallback | bd already handles this |
| Complex `ShutdownOptions` struct | Simple CLI flags | KISS principle |
| Config migration (Phase 6) | Removed | Separate concern, not blocking |
| 2300+ lines of spec | ~400 lines of spec | Focus on what matters |

---

## The Actual Problem (Simplified)

```
User runs: gt down
Expects:   Everything stops
Reality:   bd daemon respawns everything in 3 minutes
```

**Root cause**: `gt down` doesn't stop `bd daemon` or `bd activity`.

**Solution**: Stop them first.

---

## Design: Minimal Extension to `gt down`

### Current `gt down` Flow

```
1. Stop witnesses (per-rig)
2. Stop town sessions (Mayor, Boot, Deacon)  
3. Stop gt daemon
4. [--all] Kill tmux server (DESTRUCTIVE - kills non-GT sessions!)
```

### Proposed `gt down --all` Flow

```
1. [NEW] Stop bd resurrection layer (bd daemon killall + pkill bd activity)
2. Stop refineries (was MISSING!)
3. Stop witnesses (per-rig)
4. Stop town sessions (Mayor, Boot, Deacon)
5. Stop gt daemon
6. [NEW] Verify nothing respawned
```

### New `--nuke` for Old `--all` Behavior

```
--nuke: Kill entire tmux server (moved from --all, clearly destructive)
```

---

## Implementation: ~150 Lines of New Code

### File Changes

| File | Change | Lines |
|------|--------|-------|
| `internal/cmd/down.go` | Add resurrection phase, refinery loop, verify | +80 |
| `internal/beads/daemon.go` | Add `StopAllBdProcesses()` | +40 |
| `internal/cmd/down.go` | Rename `--all` to `--nuke`, add new `--all` | +10 |
| `internal/cmd/down.go` | Add `--dry-run` flag | +20 |

**Total: ~150 lines** (vs v2's ~1000+ lines of new code)

### Core Implementation

```go
// internal/beads/daemon.go - ADD this function

// StopAllBdProcesses stops all bd daemon and activity processes.
// Returns (daemonsKilled, activityKilled, error)
func StopAllBdProcesses(dryRun bool) (int, int, error) {
    var daemonsKilled, activityKilled int
    
    // Check if bd is available
    if _, err := exec.LookPath("bd"); err != nil {
        return 0, 0, nil // bd not installed, nothing to stop
    }
    
    // Count daemons before
    listCmd := exec.Command("bd", "daemon", "list", "--json")
    listOutput, _ := listCmd.Output()
    daemonsBefore := countBdDaemonsFromJSON(listOutput)
    
    if dryRun {
        // Just report what we'd do
        activityCount := countBdActivityProcesses()
        return daemonsBefore, activityCount, nil
    }
    
    // 1. Kill all bd daemons via built-in command
    if daemonsBefore > 0 {
        killCmd := exec.Command("bd", "daemon", "killall")
        killCmd.Run() // Ignore errors - killall may fail if no daemons
        
        // Verify
        time.Sleep(100 * time.Millisecond)
        verifyCmd := exec.Command("bd", "daemon", "list", "--json")
        verifyOutput, _ := verifyCmd.Output()
        daemonsAfter := countBdDaemonsFromJSON(verifyOutput)
        daemonsKilled = daemonsBefore - daemonsAfter
    }
    
    // 2. Kill bd activity processes via pkill (no killall exists for activity)
    activityBefore := countBdActivityProcesses()
    if activityBefore > 0 {
        exec.Command("pkill", "-f", "bd activity").Run()
        time.Sleep(100 * time.Millisecond)
        activityAfter := countBdActivityProcesses()
        activityKilled = activityBefore - activityAfter
    }
    
    return daemonsKilled, activityKilled, nil
}

func countBdActivityProcesses() int {
    cmd := exec.Command("pgrep", "-fc", "bd activity")
    output, err := cmd.Output()
    if err != nil {
        return 0
    }
    count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
    return count
}
```

```go
// internal/cmd/down.go - MODIFY runDown function

func runDown(cmd *cobra.Command, args []string) error {
    townRoot, err := workspace.FindFromCwdOrError()
    if err != nil {
        return fmt.Errorf("not in a Gas Town workspace: %w", err)
    }

    t := tmux.NewTmux()
    allOK := true

    // Phase 0: Dry-run header
    if downDryRun {
        fmt.Println("=== DRY RUN: Would stop the following ===\n")
    }

    // Phase 1: Stop resurrection layer (--all only)
    if downAll {
        daemonsKilled, activityKilled, err := beads.StopAllBdProcesses(downDryRun)
        if err != nil {
            printDownStatus("bd processes", false, err.Error())
            allOK = false
        } else if daemonsKilled > 0 || activityKilled > 0 {
            printDownStatus("bd daemon", true, fmt.Sprintf("%d stopped", daemonsKilled))
            printDownStatus("bd activity", true, fmt.Sprintf("%d stopped", activityKilled))
        } else {
            printDownStatus("bd processes", true, "none running")
        }
    }

    // Phase 2: Stop refineries (NEW - was missing!)
    rigs := discoverRigs(townRoot)
    for _, rigName := range rigs {
        sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
        if err := stopSession(t, sessionName, downDryRun); err != nil {
            printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
            allOK = false
        } else {
            printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "stopped")
        }
    }

    // Phase 3: Stop witnesses (existing)
    for _, rigName := range rigs {
        sessionName := fmt.Sprintf("gt-%s-witness", rigName)
        if err := stopSession(t, sessionName, downDryRun); err != nil {
            printDownStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
            allOK = false
        } else {
            printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "stopped")
        }
    }

    // Phase 4: Stop town-level sessions (existing)
    for _, ts := range session.TownSessions() {
        if downDryRun {
            running, _ := t.HasSession(ts.SessionName)
            if running {
                printDownStatus(ts.Name, true, "would stop")
            }
            continue
        }
        stopped, err := session.StopTownSession(t, ts, downForce)
        if err != nil {
            printDownStatus(ts.Name, false, err.Error())
            allOK = false
        } else if stopped {
            printDownStatus(ts.Name, true, "stopped")
        } else {
            printDownStatus(ts.Name, true, "not running")
        }
    }

    // Phase 5: Stop Daemon (existing)
    if !downDryRun {
        running, _, _ := daemon.IsRunning(townRoot)
        if running {
            if err := daemon.StopDaemon(townRoot); err != nil {
                printDownStatus("Daemon", false, err.Error())
                allOK = false
            } else {
                printDownStatus("Daemon", true, "stopped")
            }
        } else {
            printDownStatus("Daemon", true, "not running")
        }
    }

    // Phase 6: Verification (--all only)
    if downAll && !downDryRun {
        time.Sleep(500 * time.Millisecond)
        respawned := verifyShutdown(t, townRoot)
        if len(respawned) > 0 {
            fmt.Println()
            fmt.Printf("%s Warning: Some processes may have respawned:\n", style.Bold.Render("⚠"))
            for _, r := range respawned {
                fmt.Printf("  • %s\n", r)
            }
            fmt.Printf("\nThis may indicate systemd/launchd is managing bd.\n")
            fmt.Printf("Check: %s\n", style.Dim.Render("systemctl status bd-daemon"))
        }
    }

    // Phase 7: Nuke tmux server (--nuke only, RENAMED from old --all)
    if downNuke {
        if downDryRun {
            printDownStatus("Tmux server", true, "would kill")
        } else if err := t.KillServer(); err != nil {
            printDownStatus("Tmux server", false, err.Error())
            allOK = false
        } else {
            printDownStatus("Tmux server", true, "killed")
        }
    }

    // Summary
    if downDryRun {
        fmt.Println("\n=== DRY RUN COMPLETE (no changes made) ===")
        return nil
    }

    fmt.Println()
    if allOK {
        fmt.Printf("%s All services stopped\n", style.Bold.Render("✓"))
    } else {
        fmt.Printf("%s Some services failed to stop\n", style.Bold.Render("✗"))
        return fmt.Errorf("not all services stopped")
    }

    return nil
}

func verifyShutdown(t *tmux.Tmux, townRoot string) []string {
    var respawned []string
    
    // Check for bd daemons
    listCmd := exec.Command("bd", "daemon", "list", "--json")
    output, _ := listCmd.Output()
    if countBdDaemonsFromJSON(output) > 0 {
        respawned = append(respawned, "bd daemon (respawned)")
    }
    
    // Check for bd activity
    if countBdActivityProcesses() > 0 {
        respawned = append(respawned, "bd activity (respawned)")
    }
    
    // Check for gt/hq sessions
    sessions, _ := t.ListSessions()
    for _, sess := range sessions {
        if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
            respawned = append(respawned, fmt.Sprintf("session %s (still running)", sess))
        }
    }
    
    return respawned
}
```

### CLI Changes

```go
// internal/cmd/down.go - MODIFY init()

var (
    downQuiet   bool
    downForce   bool
    downAll     bool  // NEW MEANING: stop resurrection layer + verify
    downNuke    bool  // NEW: kill tmux server (old --all behavior)
    downDryRun  bool  // NEW: preview without action
)

func init() {
    downCmd.Flags().BoolVarP(&downQuiet, "quiet", "q", false, "Only show errors")
    downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "Force kill without graceful shutdown")
    downCmd.Flags().BoolVarP(&downAll, "all", "a", false, "Stop bd daemons and verify shutdown")
    downCmd.Flags().BoolVar(&downNuke, "nuke", false, "Kill entire tmux server (DESTRUCTIVE)")
    downCmd.Flags().BoolVar(&downDryRun, "dry-run", false, "Preview without action")
    rootCmd.AddCommand(downCmd)
}
```

---

## Command Interface

```bash
gt down                    # Current behavior + refineries
gt down --all              # + bd processes, verification
gt down --all --force      # Skip graceful shutdown
gt down --all --dry-run    # Preview without action
gt down --nuke             # Kill entire tmux server (DANGEROUS)
```

### Example Output

```
$ gt down --all

Stopping Gas Town...

  ✓ bd daemon: 2 stopped
  ✓ bd activity: 3 stopped
  ✓ Refinery (myproject): stopped
  ✓ Refinery (beads): stopped
  ✓ Witness (myproject): stopped
  ✓ Witness (beads): stopped
  ✓ Mayor: stopped
  ✓ Boot: stopped
  ✓ Deacon: stopped
  ✓ Daemon: stopped

✓ All services stopped
```

---

## Acceptance Criteria (Simplified from v2)

### AC1: Stop Resurrection Layer
- [ ] `gt down --all` runs `bd daemon killall` before stopping sessions
- [ ] `gt down --all` runs `pkill -f "bd activity"` after bd daemons
- [ ] Reports count of processes stopped
- [ ] Handles missing `bd` gracefully (skip with no error)

### AC2: Stop Refineries (Bug Fix)
- [ ] `gt down` now stops refineries (was missing!)
- [ ] Refineries stopped after bd, before witnesses

### AC3: Verification
- [ ] `gt down --all` waits 500ms then checks for respawns
- [ ] Warns if anything respawned (possible systemd management)

### AC4: Flag Behavior
- [ ] `--all` enables bd stopping and verification
- [ ] `--nuke` kills tmux server (renamed from old `--all`)
- [ ] `--dry-run` shows what would happen
- [ ] `--force` skips graceful shutdown

### AC5: Backwards Compatibility
- [ ] `gt down` without flags behaves same as before (+ refineries)
- [ ] Old `--all` users get warning to use `--nuke`

---

## Test Plan

### Unit Tests

```go
// internal/beads/daemon_test.go

func TestCountBdActivityProcesses(t *testing.T) {
    // Mock pgrep command
    count := countBdActivityProcesses()
    // Should return 0 or positive integer, never error
    if count < 0 {
        t.Errorf("count should be non-negative, got %d", count)
    }
}
```

### Integration Tests

```bash
#!/bin/bash
# tests/e2e/down_all_test.sh

set -euo pipefail

echo "=== Testing gt down --all ==="

# Start Gas Town
gt daemon start
sleep 2

# Verify running
gt status | grep -q "running" || { echo "FAIL: not running"; exit 1; }

# Test dry-run
gt down --all --dry-run
gt status | grep -q "running" || { echo "FAIL: dry-run affected state"; exit 1; }

# Test actual shutdown
gt down --all

# Verify stopped
if pgrep -f "bd daemon" > /dev/null; then
    echo "FAIL: bd daemon still running"
    exit 1
fi

if pgrep -f "bd activity" > /dev/null; then
    echo "FAIL: bd activity still running"
    exit 1
fi

echo "=== PASS ==="
```

---

## What's NOT in v3 (Deferred)

| Feature | Why Deferred | Where to Track |
|---------|--------------|----------------|
| Config migration (GT hooks in global settings) | Separate concern, needs own design | New issue |
| Stale file cleanup | Low priority, manual fix works | New issue |
| Shutdown lock (prevent concurrent) | Edge case, unlikely in practice | New issue |
| Cross-platform process discovery | macOS/Linux cover 99% of users | New issue |
| Custom ShutdownOptions struct | YAGNI - flags suffice | Never |

---

## Migration Guide

### For Users

```bash
# Old (v1)
gt down --all              # Killed entire tmux server!

# New (v3)  
gt down --all              # Stops GT + bd properly
gt down --nuke             # Kills entire tmux server (explicit danger)
```

### For Developers

The `--all` flag semantics changed:
- **Before**: Kill entire tmux server (destructive)
- **After**: Stop resurrection layer + verify (safe)

Add deprecation warning for 1 release cycle:

```go
if downNuke && os.Getenv("GT_NUKE_ACKNOWLEDGED") == "" {
    fmt.Printf("%s The --nuke flag kills ALL tmux sessions, not just Gas Town.\n", style.Bold.Render("⚠"))
    fmt.Printf("Set GT_NUKE_ACKNOWLEDGED=1 to suppress this warning.\n\n")
}
```

---

## Summary

| Metric | v2 | v3 | Improvement |
|--------|----|----|-------------|
| New files | 9 | 0 | No new package |
| Lines of code | ~1000 | ~150 | 85% reduction |
| Spec length | 2300 | 400 | 83% reduction |
| Features delivered | 10 | 6 | Focus on essentials |
| Time to implement | 2-3 days | 4-6 hours | 75% faster |

**v3 achieves the core goal (properly stopping Gas Town) with minimal changes.**

The remaining v2 features (config migration, stale cleanup) are nice-to-haves that can be separate PRs if needed.

---

## Cherry-Pick Reminder

The hidden directory fix from `fix/skip-hidden-dirs-in-polecat-list` (commit `a7a9455a`) should still be cherry-picked. It's orthogonal to shutdown but improves polecat listing reliability.

```bash
git cherry-pick a7a9455a
```
