---
title: "DOCS/CLI/GT SCHEDULER"
---

## gt scheduler

Manage dispatch scheduler

### Synopsis

Manage the capacity-controlled dispatch scheduler.

Subcommands:
  gt scheduler status    # Show scheduler state
  gt scheduler list      # List all scheduled beads
  gt scheduler run       # Manual dispatch trigger
  gt scheduler pause     # Pause dispatch
  gt scheduler resume    # Resume dispatch
  gt scheduler clear     # Remove beads from scheduler

Config:
  gt config set scheduler.max_polecats 5    # Enable deferred dispatch
  gt config set scheduler.max_polecats -1   # Direct dispatch (default)

```
gt scheduler [flags]
```

### Options

```
  -h, --help   help for scheduler
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt scheduler clear](../cli/gt_scheduler_clear/)	 - Remove beads from the scheduler
* [gt scheduler list](../cli/gt_scheduler_list/)	 - List all scheduled beads with titles, rig, blocked status
* [gt scheduler pause](../cli/gt_scheduler_pause/)	 - Pause all scheduler dispatch (town-wide)
* [gt scheduler resume](../cli/gt_scheduler_resume/)	 - Resume scheduler dispatch
* [gt scheduler run](../cli/gt_scheduler_run/)	 - Manually trigger scheduler dispatch
* [gt scheduler status](../cli/gt_scheduler_status/)	 - Show scheduler state: pending, capacity, active polecats

