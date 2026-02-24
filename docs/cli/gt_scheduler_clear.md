---
title: "GT SCHEDULER CLEAR"
---

## gt scheduler clear

Remove beads from the scheduler

### Synopsis

Remove beads from the scheduler by closing sling context beads.

Without --bead, removes ALL beads from the scheduler.
With --bead, removes only the specified bead.

Examples:
  gt scheduler clear              # Remove all beads from scheduler
  gt scheduler clear --bead gt-abc  # Remove specific bead

```
gt scheduler clear [flags]
```

### Options

```
      --bead string   Remove specific bead from scheduler
  -h, --help          help for clear
```

### SEE ALSO

* [gt scheduler](../cli/gt_scheduler/)	 - Manage dispatch scheduler

