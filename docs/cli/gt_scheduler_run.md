---
title: "GT SCHEDULER RUN"
---

## gt scheduler run

Manually trigger scheduler dispatch

### Synopsis

Manually trigger dispatch of scheduled work.

This dispatches scheduled beads using the same logic as the daemon heartbeat,
but can be run ad-hoc. Useful for testing or when the daemon is not running.

Examples:
  gt scheduler run                  # Dispatch using config defaults
  gt scheduler run --batch 5        # Dispatch up to 5
  gt scheduler run --dry-run        # Preview what would dispatch

```
gt scheduler run [flags]
```

### Options

```
      --batch int   Override batch size (0 = use config)
      --dry-run     Preview what would dispatch
  -h, --help        help for run
```

### SEE ALSO

* [gt scheduler](../cli/gt_scheduler/)	 - Manage dispatch scheduler

