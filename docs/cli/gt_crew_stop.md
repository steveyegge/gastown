---
title: "GT CREW STOP"
---

## gt crew stop

Stop crew workspace session(s)

### Synopsis

Stop one or more running crew workspace sessions.

If a rig name is given alone, stops all crew in that rig. Otherwise stops
the specified crew member(s).

The name can include the rig in slash format (e.g., beads/emma).
If not specified, the rig is inferred from the current directory.

Output is captured before stopping for debugging purposes (use --force
to skip capture for faster shutdown).

Examples:
  gt crew stop beads                        # Stop all crew in beads rig
  gt crew stop                              # Stop all crew (rig inferred from cwd)
  gt crew stop beads/emma                   # Stop specific crew member
  gt crew stop dave                         # Stop dave in current rig
  gt crew stop --all                        # Stop all running crew sessions
  gt crew stop dave --force                 # Stop without capturing output

```
gt crew stop [name...] [flags]
```

### Options

```
      --all          Stop all running crew sessions
      --dry-run      Show what would be stopped without stopping
      --force        Skip output capture for faster shutdown
  -h, --help         help for stop
      --rig string   Rig to use (filter when using --all)
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

