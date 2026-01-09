# Daemon/Boot/Deacon Watchdog Chain

> Autonomous health monitoring and recovery in Gas Town.

## Overview

Gas Town uses a three-tier watchdog chain for autonomous health monitoring:

```
Daemon (Go process)          ← Dumb transport, 3-min heartbeat
    │
    └─► Boot (AI agent)       ← Intelligent triage, fresh each tick
            │
            └─► Deacon (AI agent)  ← Continuous patrol, long-running
                    │
                    └─► Witnesses & Refineries  ← Per-rig agents
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

### Why Not Replace with Go Code?

The daemon could directly monitor agents without AI, but:

1. **Can't observe panes** - Go code can't interpret tmux output semantically
2. **Can't distinguish stuck vs working** - No reasoning about agent state
3. **Escalation is complex** - When to notify? When to force-restart? AI handles
   nuanced decisions better than hardcoded thresholds

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
    d.triggerPendingSpawns()        // 5. Bootstrap polecats
    d.processLifecycleRequests()    // 6. Cycle/restart requests
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
    │
    ├─ Squash wisp to digest (ephemeral → permanent)
    ├─ Write summary to molecule state
    └─ gt handoff -s "Routine cycle" -m "Details"
        │
        └─ Creates mail for next session
```

Next daemon tick:
```
Daemon → ensureDeaconRunning()
    │
    └─ Spawns fresh Deacon in gt-deacon
        │
        └─ SessionStart hook: gt mail check --inject
            │
            └─ Previous handoff mail injected
                │
                └─ Deacon reads and continues
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
- Session dead → start
- Heartbeat stale → restart
- No reasoning, just thresholds

## Fallback Chain

Multiple layers ensure recovery:

1. **Boot triage** - Intelligent observation, first line
2. **Daemon checkDeaconHeartbeat()** - Belt-and-suspenders if Boot fails
3. **Tmux-based discovery** - Daemon checks tmux sessions directly (no bead state)
4. **Human escalation** - Mail to overseer for unrecoverable states

## State Files

| File | Purpose | Updated By |
|------|---------|-----------|
| `deacon/heartbeat.json` | Deacon freshness | Deacon (each cycle) |
| `deacon/dogs/boot/.boot-running` | Boot in-progress marker | Boot spawn |
| `deacon/dogs/boot/.boot-status.json` | Boot last action | Boot triage |
| `deacon/health-check-state.json` | Agent health tracking | `gt deacon health-check` |
| `daemon/daemon.log` | Daemon activity | Daemon |
| `daemon/daemon.pid` | Daemon process ID | Daemon startup |

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

## Design Decision: Keep Separation

The issue [gt-1847v] considered three options:

### Option A: Keep Boot/Deacon Separation (CHOSEN)

- Boot is ephemeral, spawns fresh each heartbeat
- Boot runs in `gt-boot`, exits after triage
- Deacon runs in `hq-deacon`, continuous patrol
- Clear session boundaries, clear lifecycle

**Verdict**: This is the correct design. The implementation needs fixing, not the architecture.

### Option B: Merge Boot into Deacon (Rejected)

- Single `hq-deacon` session handles everything
- Deacon checks "should I be awake?" internally

**Why rejected**:
- Deacon can't observe itself (hung Deacon can't detect hang)
- Context accumulates even when idle (cost in quiet towns)
- No external watchdog means no recovery from Deacon failure

### Option C: Replace with Go Watchdog (Rejected)

- Daemon directly monitors witness/refinery
- No Boot, no Deacon AI for health checks
- AI agents only for complex decisions

**Why rejected**:
- Go code can't interpret tmux pane output semantically
- Can't distinguish "stuck" from "thinking deeply"
- Loses the intelligent triage that makes the system resilient
- Escalation decisions are nuanced (when to notify? force-restart?)

### Implementation Fixes Needed

The separation is correct; these bugs need fixing:

1. **Session confusion** (gt-sgzsb): Boot spawns in wrong session
2. **Zombie blocking** (gt-j1i0r): Daemon can't kill zombie sessions
3. ~~**Status mismatch** (gt-doih4): Bead vs tmux state divergence~~ → FIXED in gt-zecmc
4. **Ensure semantics** (gt-ekc5u): Start should kill zombies first

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
