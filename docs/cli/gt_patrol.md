---
title: "DOCS/CLI/GT PATROL"
---

## gt patrol

Patrol digest management

### Synopsis

Manage patrol cycle digests.

Patrol cycles (Deacon, Witness, Refinery) create ephemeral per-cycle digests.
This command aggregates them into permanent daily summaries.

Examples:
  gt patrol digest --yesterday  # Aggregate yesterday's patrol digests
  gt patrol digest --dry-run    # Preview what would be aggregated

### Options

```
  -h, --help   help for patrol
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt patrol digest](../cli/gt_patrol_digest/)	 - Aggregate patrol cycle digests into a daily summary bead
* [gt patrol new](../cli/gt_patrol_new/)	 - Create a new patrol wisp with config variables

