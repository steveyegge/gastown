# Gas Town

Gas Town (`gt`) is a multi-agent workspace manager that coordinates AI agents working on shared codebases.

## Overview

Gas Town manages "rigs" - workspaces where teams of AI agents collaborate on software projects. Each rig contains:

- **Crew**: Persistent human-managed agents with long-lived identity
- **Polecats**: Ephemeral workers that handle single tasks then terminate
- **Witness**: Per-rig health monitor for polecats
- **Refinery**: Merge queue processor for coordinating code integration

At the town level:
- **Mayor**: Chief of Staff for cross-rig coordination
- **Deacon**: Town-level watchdog for system health

## Key Concepts

### Rigs
A rig is a workspace containing a git repository clone with its own beads database for issue tracking. Multiple agents can work within the same rig.

### Beads
Beads is the integrated issue tracking system. Issues (bugs, tasks, epics) are stored in a local database and synced with git.

### Work Distribution
Work is "slung" to agents using `gt sling`. Agents check their "hook" for assigned work and execute autonomously.

### Molecules & Formulas
- **Formulas**: Reusable workflow templates (TOML files)
- **Molecules**: Running instances of formulas attached to specific work

### Communication
- **Mail**: Async message passing between agents
- **Nudge**: Wake sleeping agents with immediate messages
- **Decisions**: Structured human-in-the-loop gates for important choices

## Quick Reference

```bash
# Check your assigned work
gt hook

# Check mail
gt mail inbox

# Show available work
gt ready

# Assign work to an agent
gt sling <bead-id> <rig>

# Signal work complete
gt done

# Hand off to fresh session
gt handoff
```

## Architecture

```
Town (~/gt11)
├── mayor/          # Global coordinator
├── gastown/        # A rig
│   ├── .beads/     # Issue tracking database
│   ├── crew/       # Persistent agents
│   ├── polecats/   # Ephemeral workers
│   ├── refinery/   # Merge queue
│   └── witness/    # Polecat health monitor
└── beads/          # Another rig (the beads project itself)
```

## Getting Started

```bash
# Initialize a new rig
gt rig init

# Spawn a crew member
gt crew spawn <name>

# Check system status
gt status

# Run diagnostics
gt doctor
```

## Documentation

- Use `gt --help` for command reference
- Use `gt prime` for full role context
- Use `gt info` for what's new

## License

See LICENSE file.
