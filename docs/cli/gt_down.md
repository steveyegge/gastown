---
title: "GT DOWN"
---

## gt down

Stop all Gas Town services

### Synopsis

Stop Gas Town services (reversible pause).

Shutdown levels (progressively more aggressive):
  gt down                    Stop infrastructure (default)
  gt down --polecats         Also stop all polecat sessions
  gt down --all              Full shutdown with orphan cleanup
  gt down --nuke             Also kill the tmux server (DESTRUCTIVE)

Infrastructure agents stopped:
  • Refineries - Per-rig work processors
  • Witnesses  - Per-rig polecat managers
  • Mayor      - Global work coordinator
  • Boot       - Deacon's watchdog
  • Deacon     - Health orchestrator
  • Daemon     - Go background process
  • Dolt       - Shared SQL database server

This is a "pause" operation - use 'gt start' to bring everything back up.
For permanent cleanup (removing worktrees), use 'gt shutdown' instead.

Use cases:
  • Taking a break (stop token consumption)
  • Clean shutdown before system maintenance
  • Resetting the town to a clean state

```
gt down [flags]
```

### Options

```
  -a, --all        Full shutdown with orphan cleanup and verification
      --dry-run    Preview what would be stopped without taking action
  -f, --force      Force kill without graceful shutdown
  -h, --help       help for down
      --nuke       Kill entire tmux server (DESTRUCTIVE - kills non-GT sessions!)
  -p, --polecats   Also stop all polecat sessions
  -q, --quiet      Only show errors
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

