# Overseer Guide

The Overseer is a town-level patrol agent that executes assigned formulas on a configurable schedule. It runs as a persistent Claude session in its own tmux window, cycling through a duty roster of formulas.

## Quick Start

```bash
# Start the overseer
gt overseer start

# Assign a formula to its duty roster
gt patrol assign mol-overseer-patrol

# Check status
gt overseer status

# Attach to watch it work
gt overseer attach     # Detach: Ctrl-B D
```

## How It Works

The Overseer follows a linear patrol loop:

```
inbox-check -> load-duties -> execute-formulas -> context-check -> loop-or-exit
```

Each cycle:
1. **Inbox check** - Processes mail (handoffs, escalations, duty changes)
2. **Load duties** - Reads `mayor/overseer-patrol.json` for assigned formulas
3. **Execute formulas** - Creates a wisp for each formula and runs it sequentially
4. **Context check** - Monitors its own token usage
5. **Loop or exit** - Waits `patrol_interval` (default 10m) then repeats, or hands off if context is high

The Overseer is a **scheduler, not an executor**. It only runs formulas from its duty roster - it does not do freeform work.

## Commands

### Lifecycle

```bash
gt overseer start              # Start the overseer session
gt overseer start --agent NAME # Start with a specific AI agent
gt overseer stop               # Graceful shutdown
gt overseer restart             # Stop + start fresh
gt overseer status              # Check if running
gt overseer attach              # Attach to tmux session (auto-starts if not running)
```

Aliases: `gt ov start`, `gt ov at`, etc.

### Duty Roster

The duty roster controls which formulas the overseer executes each cycle. It is stored in `mayor/overseer-patrol.json`.

```bash
gt patrol duties               # List assigned formulas
gt patrol assign <formula>     # Add a formula to the roster
gt patrol unassign <formula>   # Remove a formula from the roster
```

Aliases: `gt patrol list`, `gt patrol remove`, `gt patrol rm`

All operations are idempotent - assigning a formula twice or unassigning a non-existent formula produces no error.

### Adding Custom Formulas

Formulas can come from two sources:

1. **Embedded** - Built into the `gt` binary from `internal/formula/formulas/*.formula.toml`. Provisioned to `.beads/formulas/` during `gt install`.
2. **Custom** - Drop a `.formula.toml` file directly into a rig's `.beads/formulas/` directory.

Custom formulas work the same as embedded ones for scheduling. They just won't be managed by `gt install --update-formulas`.

## Daemon Integration

The Gas Town daemon monitors the overseer:

- **On boot**: If the overseer is not running, the daemon starts it automatically.
- **Heartbeat**: Every 3 minutes, the daemon checks the overseer's tmux session. If it's dead, the daemon restarts it.
- **Crash-loop protection**: The daemon uses backoff to avoid restart storms.

This means a killed or crashed overseer session will be automatically recovered without manual intervention.

## Escalation Routing

The overseer is the final tier in the Gas Town escalation chain:

```
Agent -> Deacon -> Mayor -> Overseer
```

- **CRITICAL** severity escalations are forwarded to the overseer after the Mayor
- **Emergency** category escalations route directly to the overseer

## Configuration

### Patrol Interval

The default patrol interval is 10 minutes. It can be configured as a formula variable in `mol-overseer-patrol`:

```toml
[vars.patrol_interval]
description = "How long to wait between patrol cycles"
default = "10m"
```

### Identity

The overseer's identity is stored in `mayor/overseer.json`, auto-detected from git config, GitHub CLI, or environment on first run.

### Duty Roster File

`mayor/overseer-patrol.json`:
```json
{
  "formulas": ["mol-overseer-patrol", "mol-custom-check"]
}
```

## Status Display

The overseer appears in `gt status` with the eagle emoji:

```
🦅 overseer     ● [claude]
```

It is also included in the tmux town cycle (`Ctrl-B n/p`).

## Troubleshooting

### Overseer won't start

Check for an existing session:
```bash
tmux list-sessions | grep overseer
gt overseer status
```

Kill a stale session manually if needed:
```bash
tmux kill-session -t hq-overseer
gt overseer start
```

### Formulas not executing

1. Verify the formula exists: `gt formulas | grep <name>`
2. Check it's assigned: `gt patrol duties`
3. Attach and watch: `gt overseer attach`

### Overseer keeps restarting

Check daemon logs for crash-loop detection:
```bash
gt daemon logs | grep -i overseer
```
