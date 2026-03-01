# Dog Infrastructure: Watchdog Chain & Pool Architecture

> Autonomous health monitoring, recovery, and concurrent shutdown dances in Gas Town.

## Overview

Gas Town uses a three-tier watchdog chain for autonomous health monitoring:

```
Daemon (Go process)          <- Dumb transport, 3-min heartbeat
    |
    +-> Boot (AI agent)       <- Intelligent triage, fresh each tick
            |
            +-> Deacon (AI agent)  <- Continuous patrol, long-running
                    |
                    +-> Witnesses & Refineries  <- Per-rig agents
```

**Key insight**: The daemon is mechanical (can't reason), but health decisions need
intelligence (is the agent stuck or just thinking?). Boot bridges this gap.

## Design Rationale: Why Two Agents?

### The Problem

The daemon needs to ensure the Deacon is healthy, but:

1. **Daemon can't reason** - It's Go code following the ZFC principle (don't reason
   about other agents). It can check "is session alive?" but not "is agent stuck?"

2. **Waking costs context** - Each time you spawn an AI agent, you consume context
   tokens. In idle towns, waking Deacon every 3 minutes wastes resources.

3. **Observation requires intelligence** - Distinguishing "agent composing large
   artifact" from "agent hung on tool prompt" requires reasoning.

### The Solution: Boot as Triage

Boot is a narrow, ephemeral AI agent that:
- Runs fresh each daemon tick (no accumulated context debt)
- Makes a single decision: should Deacon wake?
- Exits immediately after deciding

This gives us intelligent triage without the cost of keeping a full AI running.

### Why Not Merge Boot into Deacon?

We could have Deacon handle its own "should I be awake?" logic, but:

1. **Deacon can't observe itself** - A hung Deacon can't detect it's hung
2. **Context accumulation** - Deacon runs continuously; Boot restarts fresh
3. **Cost in idle towns** - Boot only costs tokens when it runs; Deacon costs
   tokens constantly if kept alive

## Session Ownership

| Agent | Session Name | Location | Lifecycle |
|-------|--------------|----------|-----------|
| Daemon | (Go process) | `~/gt/daemon/` | Persistent, auto-restart |
| Boot | `gt-boot` | `~/gt/deacon/dogs/boot/` | Ephemeral, fresh each tick |
| Deacon | `hq-deacon` | `~/gt/deacon/` | Long-running, handoff loop |

**Critical**: Boot runs in `gt-boot`, NOT `hq-deacon`. This prevents Boot
from conflicting with a running Deacon session.

## Heartbeat Mechanics

### Daemon Heartbeat (3 minutes)

The daemon runs a heartbeat tick every 3 minutes:

```go
func (d *Daemon) heartbeatTick() {
    d.ensureBootRunning()           // 1. Spawn Boot for triage
    d.checkDeaconHeartbeat()        // 2. Belt-and-suspenders fallback
    d.ensureWitnessesRunning()      // 3. Witness health (checks tmux directly)
    d.ensureRefineriesRunning()     // 4. Refinery health (checks tmux directly)
    d.processLifecycleRequests()    // 5. Cycle/restart requests
    // Agent state derived from tmux, not recorded in beads (gt-zecmc)
}
```

### Deacon Heartbeat (continuous)

The Deacon updates `~/gt/deacon/heartbeat.json` at the start of each patrol cycle:

```json
{
  "timestamp": "2026-01-02T18:30:00Z",
  "cycle": 42,
  "last_action": "health-scan",
  "healthy_agents": 3,
  "unhealthy_agents": 0
}
```

### Heartbeat Freshness

| Age | State | Boot Action |
|-----|-------|-------------|
| < 5 min | Fresh | Nothing (Deacon active) |
| 5-15 min | Stale | Nudge if pending mail |
| > 15 min | Very stale | Wake (Deacon may be stuck) |

## Boot Decision Matrix

When Boot runs, it observes:
- Is Deacon session alive?
- How old is Deacon's heartbeat?
- Is there pending mail for Deacon?
- What's in Deacon's tmux pane?

Then decides:

| Condition | Action | Command |
|-----------|--------|---------|
| Session dead | START | Exit; daemon calls `ensureDeaconRunning()` |
| Heartbeat > 15 min | WAKE | `gt nudge deacon "Boot wake: check your inbox"` |
| Heartbeat 5-15 min + mail | NUDGE | `gt nudge deacon "Boot check-in: pending work"` |
| Heartbeat fresh | NOTHING | Exit silently |

## Handoff Flow

### Deacon Handoff

The Deacon runs continuous patrol cycles. After N cycles or high context:

```
End of patrol cycle:
    |
    +- Squash wisp to digest (ephemeral -> permanent)
    +- Write summary to molecule state
    +- gt handoff -s "Routine cycle" -m "Details"
        |
        +- Creates mail for next session
```

Next daemon tick:
```
Daemon -> ensureDeaconRunning()
    |
    +- Spawns fresh Deacon in gt-deacon
        |
        +- SessionStart hook: gt mail check --inject
            |
            +- Previous handoff mail injected
                |
                +- Deacon reads and continues
```

### Boot Handoff (Rare)

Boot is ephemeral - it exits after each tick. No persistent handoff needed.

However, Boot uses a marker file to prevent double-spawning:
- Marker: `~/gt/deacon/dogs/boot/.boot-running` (TTL: 5 minutes)
- Status: `~/gt/deacon/dogs/boot/.boot-status.json` (last action/result)

If the marker exists and is recent, daemon skips Boot spawn for that tick.

## Degraded Mode

When tmux is unavailable, Gas Town enters degraded mode:

| Capability | Normal | Degraded |
|------------|--------|----------|
| Boot runs | As AI in tmux | As Go code (mechanical) |
| Observe panes | Yes | No |
| Nudge agents | Yes | No |
| Start agents | tmux sessions | Direct spawn |

Degraded Boot triage is purely mechanical:
- Session dead -> start
- Heartbeat stale -> restart
- No reasoning, just thresholds

## Fallback Chain

Multiple layers ensure recovery:

1. **Boot triage** - Intelligent observation, first line
2. **Daemon checkDeaconHeartbeat()** - Belt-and-suspenders if Boot fails
3. **Tmux-based discovery** - Daemon checks tmux sessions directly (no bead state)
4. **Human escalation** - Mail to overseer for unrecoverable states

---

## Dog Pool Architecture

Boot needs to run multiple shutdown-dance molecules concurrently when multiple death
warrants are issued. All warrants need concurrent tracking, independent timeouts, and
separate outcomes.

### Design Decision: Lightweight State Machines

The shutdown-dance does NOT need Claude sessions. The dance is a deterministic
state machine:

```
WARRANT -> INTERROGATE -> EVALUATE -> PARDON|EXECUTE
```

Each step is mechanical:
1. Send a tmux message (no LLM needed)
2. Wait for timeout or response (timer)
3. Check tmux output for ALIVE keyword (string match)
4. Repeat or terminate

**Decision**: Dogs are lightweight Go routines, not Claude sessions.

### Architecture Overview

```
+-----------------------------------------------------------------+
|                             BOOT                                |
|                     (Claude session in tmux)                    |
|                                                                 |
|  +-----------------------------------------------------------+ |
|  |                      Dog Manager                           | |
|  |                                                            | |
|  |   Pool: [Dog1, Dog2, Dog3, ...]  (goroutines + state)     | |
|  |                                                            | |
|  |   allocate() -> Dog                                        | |
|  |   release(Dog)                                             | |
|  |   status() -> []DogStatus                                  | |
|  +-----------------------------------------------------------+ |
|                                                                 |
|  Boot's job:                                                    |
|  - Watch for warrants (file or event)                           |
|  - Allocate dog from pool                                       |
|  - Monitor dog progress                                         |
|  - Handle dog completion/failure                                |
|  - Report results                                               |
+-----------------------------------------------------------------+
```

### Dog Structure

```go
// Dog represents a shutdown-dance executor
type Dog struct {
    ID        string            // Unique ID (e.g., "dog-1704567890123")
    Warrant   *Warrant          // The death warrant being processed
    State     ShutdownDanceState
    Attempt   int               // Current interrogation attempt (1-3)
    StartedAt time.Time
    StateFile string            // Persistent state: ~/gt/deacon/dogs/active/<id>.json
}

type ShutdownDanceState string

const (
    StateIdle          ShutdownDanceState = "idle"
    StateInterrogating ShutdownDanceState = "interrogating"  // Sent message, waiting
    StateEvaluating    ShutdownDanceState = "evaluating"     // Checking response
    StatePardoned      ShutdownDanceState = "pardoned"       // Session responded
    StateExecuting     ShutdownDanceState = "executing"      // Killing session
    StateComplete      ShutdownDanceState = "complete"       // Done, ready for cleanup
    StateFailed        ShutdownDanceState = "failed"         // Dog crashed/errored
)

type Warrant struct {
    ID        string    // Bead ID for the warrant
    Target    string    // Session to interrogate (e.g., "gt-gastown-Toast")
    Reason    string    // Why warrant was issued
    Requester string    // Who filed the warrant
    FiledAt   time.Time
}
```

### Pool Design

**Decision**: Fixed pool of 5 dogs, configurable via environment (`GT_DOG_POOL_SIZE`).

Rationale:
- Dynamic sizing adds complexity without clear benefit
- 5 concurrent shutdown dances handles worst-case scenarios
- If pool exhausted, warrants queue (better than infinite dog spawning)
- Memory footprint is negligible (goroutines + small state files)

```go
const (
    DefaultPoolSize = 5
    MaxPoolSize     = 20
)

type DogPool struct {
    mu       sync.Mutex
    dogs     []*Dog           // All dogs in pool
    idle     chan *Dog        // Channel of available dogs
    active   map[string]*Dog  // ID -> Dog for active dogs
    stateDir string           // ~/gt/deacon/dogs/active/
}
```

### Shutdown Dance State Machine

```
                    +------------------------------------------+
                    |                                          |
                    v                                          |
    +----------------------------+                            |
    |     INTERROGATING          |                            |
    |                            |                            |
    |  1. Send health check      |                            |
    |  2. Start timeout timer    |                            |
    +-------------+--------------+                            |
                  |                                            |
                  | timeout or response                        |
                  v                                            |
    +----------------------------+                            |
    |      EVALUATING            |                            |
    |                            |                            |
    |  Check tmux output for     |                            |
    |  ALIVE keyword             |                            |
    +-------------+--------------+                            |
                  |                                            |
          +-------+-------+                                   |
          |               |                                   |
          v               v                                   |
     [ALIVE found]   [No ALIVE]                              |
          |               |                                   |
          |               | attempt < 3?                      |
          |               +-----------------------------------+
          |               | yes: attempt++, longer timeout
          |               |
          |               | no: attempt == 3
          v               v
      +---------+    +-----------+
      | PARDONED|    | EXECUTING |
      |         |    |           |
      | Cancel  |    | Kill tmux |
      | warrant |    | session   |
      +----+----+    +-----+-----+
           |               |
           +-------+-------+
                   |
                   v
          +----------------+
          |    COMPLETE    |
          |                |
          |  Write result  |
          |  Release dog   |
          +----------------+
```

### Timeout Gates

| Attempt | Timeout | Cumulative Wait |
|---------|---------|-----------------|
| 1       | 60s     | 60s             |
| 2       | 120s    | 180s (3 min)    |
| 3       | 240s    | 420s (7 min)    |

### Health Check Message

```
[DOG] HEALTH CHECK: Session {target}, respond ALIVE within {timeout}s or face termination.
Warrant reason: {reason}
Filed by: {requester}
Attempt: {attempt}/3
```

### Integration with Existing Dogs

The existing `dog` package (`internal/dog/`) manages Deacon's multi-rig helper dogs.
Those are different from shutdown-dance dogs:

| Aspect          | Helper Dogs (existing)      | Dance Dogs (new)           |
|-----------------|-----------------------------|-----------------------------|
| Purpose         | Cross-rig infrastructure    | Shutdown dance execution    |
| Sessions        | Claude sessions             | Goroutines (no Claude)      |
| Worktrees       | One per rig                 | None                        |
| Lifecycle       | Long-lived, reusable        | Ephemeral per warrant       |
| State           | idle/working                | Dance state machine         |

**Recommendation**: Use different package to avoid confusion:
- `internal/dog/` - existing helper dogs
- `internal/shutdown/` - shutdown dance pool

## Failure Handling

### Dog Crashes Mid-Dance

If a dog crashes (Boot process restarts, system crash):

1. State files persist in `~/gt/deacon/dogs/active/`
2. On Boot restart, scan for orphaned state files
3. Resume or restart based on state:

| State            | Recovery Action                    |
|------------------|------------------------------------|
| interrogating    | Restart from current attempt       |
| evaluating       | Check response, continue           |
| executing        | Verify kill, mark complete         |
| pardoned/complete| Already done, clean up             |

```go
func (p *DogPool) RecoverOrphans() error {
    files, _ := filepath.Glob(p.stateDir + "/*.json")
    for _, f := range files {
        state := loadDogState(f)
        if state.State != StateComplete && state.State != StatePardoned {
            dog := p.allocateForRecovery(state)
            go dog.Resume()
        }
    }
    return nil
}
```

### Handling Pool Exhaustion

If all dogs are busy when a new warrant arrives, the warrant is queued for
later processing. When a dog completes and is released, the queue is checked
for pending warrants.

## Directory Structure

```
~/gt/
├── daemon/
│   ├── daemon.log              # Daemon activity log
│   └── daemon.pid              # Daemon process ID
├── deacon/
│   ├── heartbeat.json          # Deacon freshness (updated each patrol cycle)
│   ├── health-check-state.json # Agent health tracking (gt deacon health-check)
│   └── dogs/
│       ├── boot/               # Boot's working directory
│       │   ├── CLAUDE.md       # Boot context
│       │   ├── .boot-running   # Boot in-progress marker (TTL: 5 min)
│       │   └── .boot-status.json # Boot last action/result
│       ├── active/             # Active dog state files
│       │   ├── dog-123.json
│       │   └── ...
│       ├── completed/          # Completed dance records (for audit)
│       │   └── dog-789.json
│       └── warrants/           # Pending warrant queue
│           └── warrant-abc.json
```

## Debugging

```bash
# Check Deacon heartbeat
cat ~/gt/deacon/heartbeat.json | jq .

# Check Boot status
cat ~/gt/deacon/dogs/boot/.boot-status.json | jq .

# View daemon log
tail -f ~/gt/daemon/daemon.log

# Manual Boot run
gt boot triage

# Manual Deacon health check
gt deacon health-check

# Dog pool status
gt dog pool status

# View active shutdown dances
gt dog dances

# View warrant queue
gt dog warrants
```

## Common Issues

### Boot Spawns in Wrong Session

**Symptom**: Boot runs in `hq-deacon` instead of `gt-boot`
**Cause**: Session name confusion in spawn code
**Fix**: Ensure `gt boot triage` specifies `--session=gt-boot`

### Zombie Sessions Block Restart

**Symptom**: tmux session exists but Claude is dead
**Cause**: Daemon checks session existence, not process health
**Fix**: Kill zombie sessions before recreating: `gt session kill hq-deacon`

### Status Shows Wrong State

**Symptom**: `gt status` shows wrong state for agents
**Cause**: Previously bead state and tmux state could diverge
**Fix**: As of gt-zecmc, status derives state from tmux directly (no bead state for
observable conditions like running/stopped). Non-observable states (stuck, awaiting-gate)
are still stored in beads.

## Summary

The watchdog chain provides autonomous recovery:

- **Daemon**: Mechanical heartbeat, spawns Boot
- **Boot**: Intelligent triage, decides Deacon fate
- **Deacon**: Continuous patrol, monitors workers

Boot exists because the daemon can't reason and Deacon can't observe itself.
The separation costs complexity but enables:

1. **Intelligent triage** without constant AI cost
2. **Fresh context** for each triage decision
3. **Graceful degradation** when tmux unavailable
4. **Multiple fallback** layers for reliability

The dog pool extends this with concurrent shutdown dances -- lightweight
Go state machines that execute warrants without consuming Claude sessions.
