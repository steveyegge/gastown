# Operational State in Gas Town

> Managing runtime state through events and labels.

## Overview

Gas Town tracks operational state changes as structured data. This document covers:
- **Events**: State transitions as beads (immutable audit trail)
- **Labels-as-state**: Fast queries via role bead labels (current state cache)

For Boot triage and degraded mode details, see [Watchdog Chain](watchdog-chain.md).

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
