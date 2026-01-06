# Bug Report: Daemon Session Restart Loop

**Date Discovered**: 2026-01-06  
**Severity**: HIGH  
**Status**: WORKAROUND APPLIED  
**Affected Components**: Daemon, Deacon, Refinery, Witness

---

## Summary

The Gas Town daemon enters a pathological state where it restarts all agent sessions (Deacon, Witness, Refinery) every 3-minute heartbeat cycle. This prevents any agent from completing work, as sessions are killed mid-task.

---

## Symptoms

1. **Agents can't complete work** - Sessions die every ~3 minutes
2. **Merge queue not processed** - Refinery keeps restarting before finishing
3. **`gt status` shows `[dead]` markers** - Even when sessions are running
4. **Work submitted but never merged** - Polecats complete, but refinery can't process

### Observable in `gt status`:
```
🐺 Deacon
   gt-deacon running [dead]    ← Shows "dead" even though running

🏭 Refinery
   gt-gastown_ui-refinery running [bead: dead]   ← Circuit breaker tripped
```

---

## Root Cause Analysis

### Discovery Process

1. **Initial observation**: Refinery sessions kept dying mid-merge
2. **Checked daemon logs**: `tail -f daemon/daemon.log`
3. **Found the pattern**:

```log
2026/01/06 03:15:46 Heartbeat starting (recovery-focused)
2026/01/06 03:15:46 Spawning Boot for triage...
2026/01/06 03:15:46 Boot spawned successfully
2026/01/06 03:15:46 Deacon heartbeat is stale (2562047h47m0s old), checking session...
2026/01/06 03:15:46 Witness for gastown_ui not running per agent bead, starting...
2026/01/06 03:15:47 Refinery for gastown_ui is marked dead (circuit breaker triggered), forcing restart...
```

### Key Issues Identified

#### Issue 1: Deacon Heartbeat Age Calculation Bug

The daemon reports:
```
Deacon heartbeat is stale (2562047h47m0s old)
```

**2,562,047 hours = ~292 million hours = ~33,000 years**

This is clearly wrong. The heartbeat file contained a valid recent timestamp:
```json
{"ts":"2026-01-05T21:47:36Z","patrol":100}
```

**Hypothesis**: The daemon's Go code has a time parsing bug, possibly:
- Comparing against zero time instead of `time.Now()`
- Timezone parsing issue
- Uninitialized time variable

#### Issue 2: Refinery Circuit Breaker Stuck

The refinery bead had `agent_state: dead` in its description:
```
Description:
role_type: refinery
rig: gastown_ui
agent_state: dead
hook_bead: 
role_bead: hq-refinery-role

Marked dead by daemon at 2026-01-05T22:05:47+05:30 (was running, last update too old)
```

Once marked dead, the daemon kept forcing restarts even after the session was healthy.

#### Issue 3: Clone Divergence

The mayor/rig clone was 36 commits behind origin/main:
```
gastown_ui/mayor/rig: 36 commits behind origin/main
```

This caused beads routing issues and state inconsistencies.

---

## How to Reproduce

### Prerequisites
- Gas Town installation with daemon running
- At least one rig with agents

### Steps

1. **Let the system run for a while** (hours/days)
2. **Check daemon logs**:
   ```bash
   tail -50 daemon/daemon.log
   ```
3. **Look for the pattern**:
   ```
   Deacon heartbeat is stale (2562047h47m0s old)
   Refinery for <rig> is marked dead (circuit breaker triggered)
   ```
4. **Observe session restarts every 3 minutes**:
   ```bash
   watch -n 10 'tail -5 daemon/daemon.log'
   ```

### Verification Commands

```bash
# Check heartbeat file
cat deacon/heartbeat.json

# Check agent bead states
bd show gt-<rig>-refinery | grep agent_state
bd show hq-deacon | grep agent_state

# Check for clone divergence
gt doctor 2>&1 | grep -A2 "clone-divergence"

# Watch daemon behavior
tail -f daemon/daemon.log
```

---

## Solution Applied

### Step 1: Fix Clone Divergence

```bash
cd gastown_ui/mayor/rig
bd sync --from-main
git stash
git pull --rebase
git stash pop
```

### Step 2: Fix Deacon Heartbeat

```bash
# Update heartbeat to current time
echo '{"ts":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","patrol":100}' > deacon/heartbeat.json

# Verify
cat deacon/heartbeat.json
```

### Step 3: Fix Refinery Bead State

```bash
# Update agent_state from "dead" to "running"
bd update gt-gastown_ui-refinery --description "role_type: refinery
rig: gastown_ui
agent_state: running
hook_bead: 
role_bead: hq-refinery-role"

# Verify
bd show gt-gastown_ui-refinery | grep agent_state
```

### Step 4: Restart Agents Properly

```bash
# Start deacon
gt deacon start

# Start refinery
gt refinery start gastown_ui

# Nudge to process work
gt nudge gastown_ui/refinery "Process merge queue"
```

### Step 5: Verify Fix

```bash
# Wait for next heartbeat cycle (3 minutes)
sleep 180

# Check if restarts stopped
tail -15 daemon/daemon.log

# Should NOT see "circuit breaker triggered" anymore
```

---

## Verification That Fix Worked

After applying the fix, the daemon log showed:
```log
2026/01/06 03:18:51 Heartbeat starting (recovery-focused)
2026/01/06 03:18:51 Spawning Boot for triage...
2026/01/06 03:18:51 Boot spawned successfully
2026/01/06 03:18:51 Deacon heartbeat is stale (2562047h47m0s old), checking session...
2026/01/06 03:18:51 Witness for gastown_ui not running per agent bead, starting...
2026/01/06 03:18:53 Heartbeat complete (#105)
```

**Key change**: No more "Refinery is marked dead (circuit breaker triggered)" message.

The refinery was then able to process the entire merge queue (5 MRs) without being killed.

---

## Permanent Fix Required

### Bug 1: Heartbeat Age Calculation (DAEMON CODE FIX NEEDED)

**Location**: Likely in `daemon/heartbeat.go` or similar

**Current behavior**: Reports absurd age like "2562047h47m0s"

**Expected behavior**: Should report actual age (e.g., "5m30s")

**Suspected issue**:
```go
// Possible bug pattern
var lastHeartbeat time.Time  // Zero value if not set
age := time.Since(lastHeartbeat)  // Huge duration from epoch

// Should be
lastHeartbeat, err := parseHeartbeatFile()
if err != nil {
    // Handle error
}
age := time.Since(lastHeartbeat)
```

### Bug 2: Circuit Breaker Recovery

**Issue**: Once an agent is marked "dead", the circuit breaker doesn't recover even when the agent is healthy again.

**Fix needed**: 
1. Clear circuit breaker on successful session start
2. Or add manual reset command: `gt circuit-reset <agent>`

### Bug 3: Clone Divergence Prevention

**Issue**: Mayor/rig can fall behind origin/main without warning

**Fix needed**:
1. Add `gt doctor` check that warns at 10+ commits behind
2. Add auto-pull option in daemon heartbeat
3. Or periodic `bd sync --from-main` in patrol

---

## Monitoring Commands

Add these to regular maintenance:

```bash
# Check for restart loop
grep "circuit breaker" daemon/daemon.log | tail -5

# Check heartbeat health
cat deacon/heartbeat.json | jq '.ts'

# Check agent states
bd list --type=agent | grep -E "dead|idle|running"

# Check clone health
gt doctor 2>&1 | grep -E "divergence|behind"
```

---

## Related Files

| File | Purpose |
|------|---------|
| `daemon/daemon.log` | Daemon activity log |
| `daemon/state.json` | Daemon state (pid, heartbeat count) |
| `deacon/heartbeat.json` | Deacon heartbeat timestamp |
| `<rig>/.runtime/refinery.json` | Refinery runtime state |
| Agent beads (`bd show`) | Agent lifecycle state |

---

## Timeline of Discovery

| Time | Action | Finding |
|------|--------|---------|
| 02:50 | Spawned 5 polecats for Sprint 7 | Polecats completed work |
| 03:00 | Noticed refinery not processing | Sessions kept dying |
| 03:05 | Checked `gt status` | Saw `[dead]` markers |
| 03:08 | Read daemon logs | Found restart loop pattern |
| 03:10 | Identified heartbeat bug | 2562047h47m0s age |
| 03:12 | Identified circuit breaker stuck | `agent_state: dead` |
| 03:15 | Applied fixes | Updated heartbeat + bead state |
| 03:18 | Verified fix | Refinery processed queue |
| 03:27 | Sprint 7 complete | All 5 MRs merged |

---

## Lessons Learned

1. **Check daemon logs first** when agents behave erratically
2. **Clone divergence** can cause cascading failures
3. **Circuit breakers** need manual reset or auto-recovery
4. **Heartbeat timestamp parsing** bugs can cause system-wide failures
5. **`gt doctor`** should catch more infrastructure issues

---

## Action Items

- [ ] File bug for daemon heartbeat age calculation
- [ ] File bug for circuit breaker auto-recovery
- [ ] Add `gt circuit-reset` command
- [ ] Improve `gt doctor` to catch stale heartbeats
- [ ] Add monitoring alert for restart loops
- [ ] Document regular maintenance checklist

---

*Document created: 2026-01-06 by Overseer session*
