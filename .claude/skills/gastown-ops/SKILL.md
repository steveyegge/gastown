---
name: gastown
description: Multi-agent Claude Code orchestration. Use for dispatching work to polecats, managing convoys, or coordinating agents. Triggers on gastown, polecat, convoy, beads.
---

# Gastown Multi-Agent Orchestration

Coordinate Claude Code agents via persistent work tracking (Beads) and structured task dispatch.

## Quick Start

```bash
bd create "Task description"         # Create issue
gt sling <prefix>-XXX --rig=<rig>    # Dispatch to polecat
gt hook                              # Check current work
gt done                              # Signal completion
```

## Core Principle

**Propulsion:** When you find work on your hook, EXECUTE immediately. No confirmation, no waiting.

## Key Concepts

| Concept | Purpose |
|---------|---------|
| **Town** | HQ directory (default: `~/gt`) where all rigs live |
| **Rig** | Project container (git repo + agents) |
| **Polecat** | Worker agent with a persistent name (furiosa, nux, max, etc.) |
| **Hook** | Agent's current work assignment |
| **Beads** | Git-backed issue tracking |
| **Convoy** | Grouped tasks |

## Status Check (Is my polecat done?)

```bash
export BEADS_DIR="$HOME/gt/<rig>/.beads"
bd list                              # Shows "hooked @<rig>/polecats/<name>"
gh pr list --state all               # Check if PR merged
bd close <issue-id>                  # Close when complete
tmux ls                              # Shows gt-<rig>-<name> sessions
```

Example: "Is furiosa merged?" → `bd list` shows `<prefix>-XXX hooked @<rig>/polecats/furiosa` → check `gh pr list` for that work

## References

- [Setup Guide](references/setup.md) - Installation and initialization
- [Commands](references/commands.md) - Full gt/bd command reference
- [Workflows](references/workflows.md) - Multi-agent patterns
- [Mayor Context](references/mayor-context.md) - Full Mayor documentation
- [Architecture](references/architecture.md) - Directory structure and config
