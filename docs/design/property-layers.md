# Property Layers: Multi-Level Configuration

> Implementation guide for Gas Town's configuration system.
> Created: 2025-01-06

## Overview

Gas Town uses a layered property system for configuration. Properties are
looked up through multiple layers, with earlier layers overriding later ones.
This enables both local control and global coordination.

## The Four Layers

```
┌─────────────────────────────────────────────────────────────┐
│ 1. WISP LAYER (transient, town-local)                       │
│    Location: <rig>/.beads-wisp/config/                      │
│    Synced: Never                                            │
│    Use: Temporary local overrides                           │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. RIG BEAD LAYER (persistent, synced globally)             │
│    Location: <rig>/.beads/ (rig identity bead labels)       │
│    Synced: Via git (all clones see it)                      │
│    Use: Project-wide operational state                      │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. TOWN DEFAULTS                                            │
│    Location: ~/gt/config.json or ~/gt/.beads/               │
│    Synced: N/A (per-town)                                   │
│    Use: Town-wide policies                                  │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. SYSTEM DEFAULTS (compiled in)                            │
│    Use: Fallback when nothing else specified                │
└─────────────────────────────────────────────────────────────┘
```

## Lookup Behavior

### Override Semantics (Default)

For most properties, the first non-nil value wins:

```go
func GetConfig(key string) interface{} {
    if val := wisp.Get(key); val != nil {
        if val == Blocked { return nil }
        return val
    }
    if val := rigBead.GetLabel(key); val != nil {
        return val
    }
    if val := townDefaults.Get(key); val != nil {
        return val
    }
    return systemDefaults[key]
}
```

### Stacking Semantics (Integers)

For integer properties, values from wisp and bead layers **add** to the base:

```go
func GetIntConfig(key string) int {
    base := getBaseDefault(key)    // Town or system default
    beadAdj := rigBead.GetInt(key) // 0 if missing
    wispAdj := wisp.GetInt(key)    // 0 if missing
    return base + beadAdj + wispAdj
}
```

This enables temporary adjustments without changing the base value.

### Blocking Inheritance

You can explicitly block a property from being inherited:

```bash
gt rig config set gastown auto_restart --block
```

This creates a "blocked" marker in the wisp layer. Even if the rig bead
or defaults say `auto_restart: true`, the lookup returns nil.

## Rig Identity Beads

Each rig has an identity bead for operational state:

```yaml
id: gt-rig-gastown
type: rig
name: gastown
repo: git@github.com:steveyegge/gastown.git
prefix: gt

labels:
  - status:operational
  - priority:normal
```

These beads sync via git, so all clones of the rig see the same state.

## Two-Level Rig Control

### Level 1: Park (Local, Ephemeral)

```bash
gt rig park gastown      # Stop services, daemon won't restart
gt rig unpark gastown    # Allow services to run
```

- Stored in wisp layer (`.beads-wisp/config/`)
- Only affects this town
- Disappears on cleanup
- Use: Local maintenance, debugging

### Level 2: Dock (Global, Persistent)

```bash
gt rig dock gastown      # Set status:docked label on rig bead
gt rig undock gastown    # Remove label
```

- Stored on rig identity bead
- Syncs to all clones via git
- Permanent until explicitly changed
- Use: Project-wide maintenance, coordinated downtime

### Daemon Behavior

The daemon checks both levels before auto-restarting:

```go
func shouldAutoRestart(rig *Rig) bool {
    status := rig.GetConfig("status")
    if status == "parked" || status == "docked" {
        return false
    }
    return true
}
```

## Configuration Keys

| Key | Type | Behavior | Description |
|-----|------|----------|-------------|
| `status` | string | Override | operational/parked/docked |
| `auto_restart` | bool | Override | Daemon auto-restart behavior |
| `max_polecats` | int | Override | Maximum concurrent polecats |
| `priority_adjustment` | int | **Stack** | Scheduling priority modifier |
| `maintenance_window` | string | Override | When maintenance allowed |
| `dnd` | bool | Override | Do not disturb mode |

## Commands

### View Configuration

```bash
gt rig config show gastown           # Show effective config (all layers)
gt rig config show gastown --layer   # Show which layer each value comes from
```

### Set Configuration

```bash
# Set in wisp layer (local, ephemeral)
gt rig config set gastown key value

# Set in bead layer (global, permanent)
gt rig config set gastown key value --global

# Block inheritance
gt rig config set gastown key --block

# Clear from wisp layer
gt rig config unset gastown key
```

### Rig Lifecycle

```bash
gt rig park gastown          # Local: stop + prevent restart
gt rig unpark gastown        # Local: allow restart

gt rig dock gastown          # Global: mark as offline
gt rig undock gastown        # Global: mark as operational

gt rig status gastown        # Show current state
```

## Examples

### Temporary Priority Boost

```bash
# Base priority: 0 (from defaults)
# Give this rig temporary priority boost for urgent work

gt rig config set gastown priority_adjustment 10

# Effective priority: 0 + 10 = 10
# When done, clear it:

gt rig config unset gastown priority_adjustment
```

### Local Maintenance

```bash
# I'm upgrading the local clone, don't restart services
gt rig park gastown

# ... do maintenance ...

gt rig unpark gastown
```

### Project-Wide Maintenance

```bash
# Major refactor in progress, all clones should pause
gt rig dock gastown

# Syncs via git - other towns see the rig as docked
bd sync

# When done:
gt rig undock gastown
bd sync
```

### Block Auto-Restart Locally

```bash
# Rig bead says auto_restart: true
# But I'm debugging and don't want that here

gt rig config set gastown auto_restart --block

# Now auto_restart returns nil for this town only
```

## Implementation Notes

### Wisp Storage

Wisp config stored in `.beads-wisp/config/<rig>.json`:

```json
{
  "rig": "gastown",
  "values": {
    "status": "parked",
    "priority_adjustment": 10
  },
  "blocked": ["auto_restart"]
}
```

### Rig Bead Labels

Rig operational state stored as labels on the rig identity bead:

```bash
bd label add gt-rig-gastown status:docked
bd label remove gt-rig-gastown status:docked
```

### Daemon Integration

The daemon's lifecycle manager checks config before starting services:

```go
func (d *Daemon) maybeStartRigServices(rig string) {
    r := d.getRig(rig)

    status := r.GetConfig("status")
    if status == "parked" || status == "docked" {
        log.Info("Rig %s is offline, skipping auto-start", rig)
        return
    }

    d.ensureWitness(rig)
    d.ensureRefinery(rig)
}
```

## Related Documents

- `~/gt/docs/hop/PROPERTY-LAYERS.md` - Strategic architecture
- `wisp-architecture.md` - Wisp system design
- `agent-as-bead.md` - Agent identity beads (similar pattern)
