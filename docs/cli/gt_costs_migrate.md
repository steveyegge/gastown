---
title: "DOCS/CLI/GT COSTS MIGRATE"
---

## gt costs migrate

Migrate legacy session.ended beads to the new log-file architecture

### Synopsis

Migrate legacy session.ended event beads to the new cost tracking system.

This command handles the transition from the old architecture (where each
session.ended event was a permanent bead) to the new log-file-based system.

The migration:
1. Finds all open session.ended event beads (should be none if auto-close worked)
2. Closes them with reason "migrated to log-file architecture"

Legacy beads remain in the database for historical queries but won't interfere
with the new log-file-based cost tracking.

Examples:
  gt costs migrate            # Migrate legacy beads
  gt costs migrate --dry-run  # Preview what would be migrated

```
gt costs migrate [flags]
```

### Options

```
      --dry-run   Preview what would be migrated without making changes
  -h, --help      help for migrate
```

### SEE ALSO

* [gt costs](../cli/gt_costs/)	 - Show costs for running Claude sessions

