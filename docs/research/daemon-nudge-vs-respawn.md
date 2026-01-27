# Research: Can Daemon Nudge Boot Instead of Killing/Respawning?

**Research ID**: hq-ran5
**Date**: 2026-01-13
**Context**: Follow-up to hq-to8h (Daemon should nudge boot/deacon on heartbeat, not skip)

## Executive Summary

**YES**, the daemon can and should nudge existing sessions instead of always respawning. However, the current issue is NOT about choosing between nudge vs respawn - it's that **the daemon does neither** when Boot is already running.

### The Core Problem

When `ensureBootRunning()` detects an existing Boot tmux session, it logs "Boot already running, skipping spawn" and returns immediately. This means:

1. No new Boot triage runs
2. No one checks if Deacon heartbeat is stale
3. Deacon can be stuck for hours (7.5 hours observed in hq-to8h)
4. System appears healthy but is actually idle

**Root cause**: `Boot.IsRunning()` only checks if the tmux session exists (`HasSession`), not if Claude is actually working in it (`IsClaudeRunning`).

## Current Architecture

### Watchdog Chain
```
Daemon (3-min heartbeat)
    ↓
Boot (ephemeral triage agent)
    ↓
Deacon (long-running patrol agent)
    ↓
Witnesses/Refineries (per-rig agents)
```

### Boot Lifecycle

**Location**: `internal/daemon/daemon.go:246-278`

```go
func (d *Daemon) ensureBootRunning() {
    b := boot.New(d.config.TownRoot)

    if b.IsRunning() {
        d.logger.Println("Boot already running, skipping spawn")
        return  // ← THE PROBLEM: No triage happens!
    }

    // If not running, spawn Boot to do triage...
}
```

**Boot is designed to**:
- Run fresh on each daemon tick (3 minutes)
- Observe system state (Deacon heartbeat, session health, mail)
- Decide action: START, WAKE, NUDGE, INTERRUPT, or NOTHING
- Execute action and exit

### Deacon Heartbeat Freshness

**Location**: `internal/deacon/heartbeat.go:88-116`

- **Fresh** (< 5 min): Actively working
- **Stale** (5-15 min): Possibly long operation
- **Very Stale** (> 15 min): Should be poked

## Research Questions Answered

### 1. Can the daemon just nudge an existing boot/deacon session to keep it alive?

**YES**. Infrastructure already exists:

**For Boot**: Not applicable - Boot is ephemeral by design (runs fresh each tick)

**For Deacon**: Multiple nudge mechanisms exist:
- `tmux.NudgeSession(sessionName, message)` - Send prompt to running session
- `gt nudge deacon "message"` - CLI wrapper
- Boot's decision matrix already defines when to nudge:
  - Idle 5-15 min + has mail → NUDGE
  - Idle > 15 min → WAKE (stronger nudge)

**Current usage**: Boot's triage logic (when it DOES run) already uses nudging:
```bash
# From boot.md.tmpl
gt nudge deacon "Boot check-in: you have pending work"  # NUDGE
gt nudge deacon "Boot wake: check your inbox"           # WAKE
```

### 2. What are the trade-offs between nudge vs kill/respawn?

#### NUDGE (Keep existing session alive)

**Pros**:
- Preserves conversation context/history
- Less disruptive to legitimate work-in-progress
- Lower computational cost (no new Claude session spawn)
- Maintains agent's accumulated understanding of current work
- Faster response (just needs to wake up, not reinitialize)

**Cons**:
- May not fix root cause if agent is genuinely stuck
- Could perpetuate accumulated confusion/context bloat
- Agent might be in an error state that nudging won't fix
- False positive nudges disrupt legitimate long-running operations

**Best for**:
- Agent is idle but healthy
- Recent successful work (fresh heartbeat within 5-15 min)
- Clear trigger (new mail, pending work)
- First intervention attempt

#### KILL/RESPAWN (Fresh session)

**Pros**:
- Guaranteed fresh state - no accumulated confusion
- Fixes stuck states that nudging can't resolve
- Clean slate for new work assignments
- Forces re-evaluation of current priorities

**Cons**:
- Loses all conversation context/history
- More expensive (new Claude API session)
- More disruptive if agent was doing legitimate work
- Interrupts mid-operation work
- Higher latency (session spawn + initialization)

**Best for**:
- Agent truly stuck (heartbeat > 15-30 min)
- Session dead (tmux session exists but Claude not running)
- After nudge escalation failed
- Suspected accumulated confusion/context issues
- Critical state requiring guaranteed fresh start

#### Recommended Graduated Escalation

This is ALREADY implemented in Boot's decision matrix:

| Stage | Condition | Action | Method |
|-------|-----------|--------|--------|
| 0 | Fresh (< 5 min) | NOTHING | Let it work |
| 1 | Stale (5-15 min) + mail | NUDGE | Gentle prompt |
| 2 | Very stale (> 15 min) | WAKE | Stronger nudge with escape |
| 3 | Stuck (errors visible) | INTERRUPT | Mail requesting restart |
| 4 | Session dead | START | Kill and respawn |

### 3. Is there a recovery mechanism if nudges fail?

**YES**. Boot implements a graduated escalation path:

**Stage 1: NUDGE** (soft intervention)
```bash
gt nudge deacon "Boot check-in: you have pending work"
```
- For stale heartbeat (5-15 min) when mail exists
- Non-disruptive prompt
- Agent can ignore if legitimately busy

**Stage 2: WAKE** (stronger intervention)
```bash
# Send escape + nudge
gt nudge deacon "Boot wake: check your inbox"
```
- For very stale heartbeat (> 15 min)
- Includes escape sequence to interrupt current operation
- Forces attention

**Stage 3: INTERRUPT** (request manual intervention)
```bash
gt mail send <rig>/deacon -s "Boot: session appears stuck" -m "..."
```
- For stuck state (visible errors, long-running with no progress)
- Creates mail bead for human/Witness review
- Doesn't force restart (may be legitimate long operation)

**Stage 4: START** (full restart)
```go
// Boot exits, logs detection
// Daemon calls ensureDeaconRunning() on next tick
d.ensureDeaconRunning()
```
- Only when session is dead (tmux session missing)
- Daemon handles the actual restart
- Guaranteed fresh state

**Gap in current implementation**: If Boot itself is stuck/zombie, the daemon never progresses past Stage 0 because it keeps skipping Boot spawn.

### 4. What's the correct lifecycle management for boot/deacon?

#### Current Implementation Issues

**Boot lifecycle** (`internal/boot/boot.go:82-92`):
```go
func (b *Boot) IsRunning() bool {
    return b.IsSessionAlive()  // Only checks session exists!
}

func (b *Boot) IsSessionAlive() bool {
    has, err := b.tmux.HasSession(SessionName)
    return err == nil && has
}
```

**Problem**: Checks session existence, not if Claude is actually running.

**Result**:
- Boot spawns, completes triage, but tmux session lingers
- Next daemon tick: "Boot already running, skipping spawn"
- No new triage happens
- Deacon heartbeat goes stale unnoticed

#### Proposed Solution: Use IsClaudeRunning()

**Infrastructure already exists**: `tmux.IsClaudeRunning()` is used elsewhere in the codebase:
- `internal/daemon/lifecycle.go:812,903`
- `internal/witness/manager.go:117,134`
- `internal/deacon/manager.go:63`

**Location**: `internal/tmux/tmux.go:642-672`
```go
func (t *Tmux) IsClaudeRunning(session string) bool {
    // Checks pane command for: "node", "claude", or version pattern
    // Also checks child processes if pane is a shell
    return ...
}
```

**Recommendation**: Change Boot's `IsRunning()` to check if Claude is actually working:

```go
func (b *Boot) IsRunning() bool {
    // Check if session exists AND Claude is running in it
    if !b.IsSessionAlive() {
        return false
    }
    return b.tmux.IsClaudeRunning(SessionName)
}
```

#### Alternative: TTL-Based Session Cleanup

If we want to keep the current check but handle long-running Boot instances:

```go
func (b *Boot) IsRunning() bool {
    if !b.IsSessionAlive() {
        return false
    }

    // Check if Boot has been running too long (>5 min)
    status, _ := b.LoadStatus()
    if status != nil && status.Running {
        age := time.Since(status.StartedAt)
        if age > 5*time.Minute {
            // Stale Boot session - kill and allow respawn
            _ = b.tmux.KillSession(SessionName)
            return false
        }
    }

    return true
}
```

#### Alternative: Daemon Direct Check

Bypass Boot entirely when Boot is already running:

```go
func (d *Daemon) ensureBootRunning() {
    b := boot.New(d.config.TownRoot)

    if b.IsRunning() {
        // Boot is running, but check Deacon health directly
        hb := deacon.ReadHeartbeat(d.config.TownRoot)
        if hb.ShouldPoke() {
            // Deacon is stuck, nudge it directly
            _ = d.tmux.NudgeSession(d.getDeaconSessionName(),
                "Daemon wake: heartbeat very stale")
        }
        return
    }

    // Normal Boot spawn...
}
```

#### Recommended Lifecycle Management

**For Boot (ephemeral watchdog)**:
1. Check `IsSessionAlive() && IsClaudeRunning()` before skipping spawn
2. If session exists but Claude not running → kill stale session, spawn fresh
3. Add 5-minute TTL check as safety net
4. Boot should exit cleanly after triage (already does this)

**For Deacon (long-running patrol)**:
1. Keep existing graduated escalation (NUDGE → WAKE → INTERRUPT → START)
2. Daemon checks heartbeat freshness on each tick
3. Daemon nudges Deacon if heartbeat stale AND Boot not handling it
4. Only kill/respawn when session dead or after escalation exhausted

**For Daemon heartbeat cycle**:
1. Every 3 minutes: check Boot health
2. If Boot stale/zombie: kill and spawn fresh
3. If Boot healthy: let it handle Deacon triage
4. If Deacon heartbeat very stale AND Boot skipped: nudge Deacon directly

## Code Locations

### Key Files
- `internal/daemon/daemon.go:246-278` - `ensureBootRunning()` with skip logic
- `internal/boot/boot.go:82-92` - Boot `IsRunning()` check (needs fix)
- `internal/deacon/heartbeat.go:88-116` - Heartbeat freshness thresholds
- `internal/tmux/tmux.go:642-672` - `IsClaudeRunning()` implementation
- `internal/templates/roles/boot.md.tmpl:76-84` - Boot decision matrix

### Related Issues
- `hq-to8h` - Original bug report: daemon skips spawn but never nudges
- `hq-ran5` - This research task

## Recommendations

### Immediate Fix (Minimal Change)

**File**: `internal/boot/boot.go`

Change `IsRunning()` to use `IsClaudeRunning()`:

```go
// IsRunning checks if Boot is currently running.
// Queries tmux directly for observable reality (ZFC principle).
func (b *Boot) IsRunning() bool {
    // Check session exists AND Claude is actually running in it
    if !b.IsSessionAlive() {
        return false
    }
    return b.tmux.IsClaudeRunning(SessionName)
}
```

**Impact**:
- Boot will only skip spawn if Claude is ACTIVELY running triage
- Zombie Boot sessions (Claude exited but tmux still exists) will be cleaned up
- Daemon will spawn fresh Boot if previous one completed but session lingered

### Enhanced Fix (Add TTL Safety Net)

Add status-based TTL check to catch long-running Boot instances:

```go
func (b *Boot) IsRunning() bool {
    if !b.IsSessionAlive() {
        return false
    }

    // If Claude not running, session is a zombie
    if !b.tmux.IsClaudeRunning(SessionName) {
        return false
    }

    // Safety net: Boot shouldn't run longer than 5 minutes
    status, _ := b.LoadStatus()
    if status != nil && status.Running {
        if time.Since(status.StartedAt) > 5*time.Minute {
            // Kill long-running Boot session
            _ = b.tmux.KillSession(SessionName)
            return false
        }
    }

    return true
}
```

### Future Enhancement (Daemon Direct Nudge)

For more robustness, add daemon-level Deacon health check:

```go
func (d *Daemon) ensureBootRunning() {
    b := boot.New(d.config.TownRoot)

    if b.IsRunning() {
        // Boot is handling triage, but verify Deacon isn't critically stuck
        hb := deacon.ReadHeartbeat(d.config.TownRoot)

        // If heartbeat is VERY stale (>30 min) and Deacon session exists
        if hb != nil && hb.Age() > 30*time.Minute {
            sessionName := d.getDeaconSessionName()
            if has, _ := d.tmux.HasSession(sessionName); has {
                // Critical: Deacon is stuck and Boot isn't fixing it
                // Escalate with direct nudge
                _ = d.tmux.NudgeSession(sessionName,
                    "ESCALATION: Daemon direct wake (heartbeat critical)")
            }
        }

        return
    }

    // Normal Boot spawn...
}
```

## Conclusion

The daemon CAN and SHOULD use nudging as the primary intervention mechanism, with kill/respawn as escalation. The current system already implements this via Boot's decision matrix.

**The bug is not in the nudge-vs-respawn logic** - it's in the Boot liveness check preventing Boot from running at all.

**Fix priority**:
1. **Critical**: Change `Boot.IsRunning()` to use `IsClaudeRunning()` (prevents zombie session false positives)
2. **Important**: Add TTL safety net (prevents runaway Boot sessions)
3. **Enhancement**: Add daemon direct-nudge fallback (defense in depth)

**Expected behavior after fix**:
- Daemon spawns Boot every 3 minutes (unless Claude actively working in Boot session)
- Boot checks Deacon heartbeat freshness
- Boot nudges/wakes Deacon if stale (graduated escalation)
- Deacon responds to nudges and updates heartbeat
- Only respawns when session dead or escalation exhausted
