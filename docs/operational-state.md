# Operational State in Gas Town

> Managing runtime state, degraded modes, and the Boot triage system.

## Overview

Gas Town needs to track operational state: Is the Deacon's patrol muted? Is the
system in degraded mode? When did state change, and why?

This document covers:
- **Events**: State transitions as beads
- **Labels-as-state**: Fast queries via role bead labels
- **Boot**: The dog that triages the Deacon
- **Degraded mode**: Operating without tmux

## Events: State Transitions as Data

Operational state changes are recorded as event beads. Each event captures:
- **What** changed (`event_type`)
- **Who** caused it (`actor`)
- **What** was affected (`target`)
- **Context** (`payload`)
- **When** (`created_at`)

### Event Types

| Event Type | Description | Payload |
|------------|-------------|---------|
| `patrol.muted` | Patrol cycle disabled | `{reason, until?}` |
| `patrol.unmuted` | Patrol cycle re-enabled | `{reason?}` |
| `agent.started` | Agent session began | `{session_id?}` |
| `agent.stopped` | Agent session ended | `{reason, outcome?}` |
| `mode.degraded` | System entered degraded mode | `{reason}` |
| `mode.normal` | System returned to normal | `{}` |

### Creating Events

```bash
# Mute deacon patrol
bd create --type=event --event-type=patrol.muted \
  --actor=human:overseer --target=agent:deacon \
  --payload='{"reason":"fixing convoy deadlock","until":"gt-abc1"}'

# System entered degraded mode
bd create --type=event --event-type=mode.degraded \
  --actor=system:daemon --target=rig:greenplace \
  --payload='{"reason":"tmux unavailable"}'
```

### Querying Events

```bash
# Recent events for an agent
bd list --type=event --target=agent:deacon --limit=10

# All patrol state changes
bd list --type=event --event-type=patrol.muted
bd list --type=event --event-type=patrol.unmuted

# Events in the activity feed
bd activity --follow --type=event
```

## Labels-as-State Pattern

Events capture the full history. Labels cache the current state for fast queries.

### Convention

Labels use `<dimension>:<value>` format:
- `patrol:muted` / `patrol:active`
- `mode:degraded` / `mode:normal`
- `status:idle` / `status:working`

### State Change Flow

1. Create event bead (full context, immutable)
2. Update role bead labels (current state cache)

```bash
# Mute patrol
bd create --type=event --event-type=patrol.muted ...
bd update role-deacon --add-label=patrol:muted --remove-label=patrol:active

# Unmute patrol
bd create --type=event --event-type=patrol.unmuted ...
bd update role-deacon --add-label=patrol:active --remove-label=patrol:muted
```

### Querying Current State

```bash
# Is deacon patrol muted?
bd show role-deacon | grep patrol:

# All agents with muted patrol
bd list --type=role --label=patrol:muted

# All agents in degraded mode
bd list --type=role --label=mode:degraded
```

## Boot: The Deacon's Watchdog

> See [Watchdog Chain](watchdog-chain.md) for the complete Daemon/Boot/Deacon
> architecture and design rationale.

Boot is a dog (Deacon helper) that triages the Deacon's health. The daemon pokes
Boot instead of the Deacon directly, centralizing the "when to wake" decision in
an agent that can reason about it.

### Why Boot?

The daemon is dumb transport (ZFC principle). It can't decide:
- Is the Deacon stuck or just thinking?
- Should we interrupt or let it continue?
- Is the system in a state where nudging would help?

Boot is an agent that can observe and decide.

### Boot's Lifecycle

```
Daemon tick
    │
    ├── Check: Is Boot already running? (marker file)
    │   └── Yes + recent: Skip this tick
    │
    └── Spawn Boot (fresh session each time)
        │
        └── Boot runs triage molecule
            ├── Observe (wisps, mail, git state, tmux panes)
            ├── Decide (start/wake/nudge/interrupt/nothing)
            ├── Act
            ├── Clean inbox (discard stale handoffs)
            └── Handoff (or exit in degraded mode)
```

### Boot is Always Fresh

Boot restarts on each daemon tick. This is intentional:
- Narrow scope makes restarts cheap
- Fresh context avoids accumulated confusion
- Handoff mail provides continuity without session persistence
- No keepalive needed

### Boot's Decision Guidance

Agents may take several minutes on legitimate work - composing artifacts, running
tools, deep analysis. Ten minutes or more in edge cases.

To assess whether an agent is stuck:
1. Check the agent's last reported activity (recent wisps, mail sent, git commits)
2. Observe the tmux pane output over a 30-second window
3. Look for signs of progress vs. signs of hanging (tool prompt, error loop, silence)

Agents work in small steps with feedback. Most tasks complete in 2-3 minutes, but
task nature matters.

**Boot's options (increasing disruption):**
- Let them continue (if progress is evident)
- `gt nudge <agent>` (gentle wake signal)
- Escape + chat (interrupt and ask what's happening)
- Request process restart (last resort, for true hangs)

**Common false positives:**
- Tool waiting for user confirmation
- Long-running test suite
- Large file read/write operations

### Boot's Location

```
~/gt/deacon/dogs/boot/
```

Session name: `gt-boot`

Created/maintained by `bd doctor`.

### Boot Commands

```bash
# Check Boot status
gt dog status boot

# Manual Boot run (debugging)
gt dog call boot

# Prime Boot with context
gt dog prime boot
```

## Degraded Mode

Gas Town can operate without tmux, with reduced capabilities.

### Detection

The daemon detects degraded mode mechanically and passes it to agents:

```bash
GT_DEGRADED=true  # Set by daemon when tmux unavailable
```

Boot and other agents check this environment variable.

### What Changes in Degraded Mode

| Capability | Normal | Degraded |
|------------|--------|----------|
| Observe tmux panes | Yes | No |
| Interactive interrupt | Yes | No |
| Session management | Full | Limited |
| Agent spawn | tmux sessions | Direct spawn |
| Boot lifecycle | Handoff | Exit |

### Agents in Degraded Mode

In degraded mode, agents:
- Cannot observe other agents' pane output
- Cannot interactively interrupt stuck agents
- Focus on beads/git state observation only
- Report anomalies but can't fix interactively

Boot specifically:
- Runs to completion and exits (no handoff)
- Limited to: start deacon, file beads, mail overseer
- Cannot: observe panes, nudge, interrupt

### Recording Degraded Mode

```bash
# System entered degraded mode
bd create --type=event --event-type=mode.degraded \
  --actor=system:daemon --target=rig:greenplace \
  --payload='{"reason":"tmux unavailable"}'

bd update role-greenplace --add-label=mode:degraded --remove-label=mode:normal
```

## Configuration vs State

| Type | Storage | Example |
|------|---------|---------|
| **Static config** | TOML files | Daemon tick interval |
| **Operational state** | Beads (events + labels) | Patrol muted |
| **Runtime flags** | Marker files | `.deacon-disabled` |

Static config rarely changes and doesn't need history.
Operational state changes at runtime and benefits from audit trail.
Marker files are fast checks that can trigger deeper beads queries.

## Commands Summary

```bash
# Create operational event
bd create --type=event --event-type=<type> \
  --actor=<entity> --target=<entity> --payload='<json>'

# Update state label
bd update <role-bead> --add-label=<dim>:<val> --remove-label=<dim>:<old>

# Query current state
bd list --type=role --label=<dim>:<val>

# Query state history
bd list --type=event --target=<entity>

# Boot management
gt dog status boot
gt dog call boot
gt dog prime boot
```

---

*Events are the source of truth. Labels are the cache.*
