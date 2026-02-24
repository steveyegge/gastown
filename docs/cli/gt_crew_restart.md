---
title: "DOCS/CLI/GT CREW RESTART"
---

## gt crew restart

Kill and restart crew workspace session(s)

### Synopsis

Kill the tmux session and restart fresh with Claude.

Useful when a crew member gets confused or needs a clean slate.
Unlike 'refresh', this does NOT send handoff mail - it's a clean start.

The command will:
1. Kill existing tmux session if running
2. Start fresh session with Claude
3. Run gt prime to reinitialize context

Use --all to restart all running crew sessions across all rigs.

Examples:
  gt crew restart dave                  # Restart dave's session
  gt crew restart dave emma fred        # Restart multiple
  gt crew restart beads/grip beads/fang # Restart from specific rig
  gt crew rs emma                       # Same, using alias
  gt crew restart --all                 # Restart all running crew sessions
  gt crew restart --all --rig beads     # Restart all crew in beads rig
  gt crew restart --all --dry-run       # Preview what would be restarted

```
gt crew restart [name...] [flags]
```

### Options

```
      --all          Restart all running crew sessions
      --dry-run      Show what would be restarted without restarting
  -h, --help         help for restart
      --rig string   Rig to use (filter when using --all)
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

