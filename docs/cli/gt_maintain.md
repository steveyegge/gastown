---
title: "GT MAINTAIN"
---

## gt maintain

Run full Dolt maintenance (reap + flatten + gc)

### Synopsis

Run the full Dolt maintenance pipeline in a single command.

All operations run via SQL on the running server — no downtime needed.

This encapsulates the maintenance procedure:
  1. Backup all databases (dolt backup sync)
  2. Reap closed wisps from each database
  3. Flatten databases over commit threshold
  4. Run dolt_gc() on each database

Use --force for non-interactive mode (daemon/cron), or run interactively
to review the plan before proceeding.

Examples:
  gt maintain                # Interactive (shows plan, asks confirmation)
  gt maintain --force        # Non-interactive (daemon/cron use)
  gt maintain --dry-run      # Preview what would happen
  gt maintain --threshold 50 # Custom commit threshold

```
gt maintain [flags]
```

### Options

```
      --dry-run         Preview without making changes
      --force           Non-interactive mode (skip confirmation)
  -h, --help            help for maintain
      --threshold int   Commit count threshold for flatten (default 100)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

