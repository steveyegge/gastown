---
title: "DOCS/CLI/GT DOLT SYNC"
---

## gt dolt sync

Push Dolt databases to DoltHub remotes

### Synopsis

Push all local Dolt databases to their configured DoltHub remotes.

This command automates the tedious process of pushing each database individually:
  1. Stops the Dolt server (required for CLI push)
  2. Optionally purges closed ephemeral beads (--gc)
  3. Iterates databases in .dolt-data/
  4. For each database with a configured remote, runs dolt push
  5. Reports success/failure per database
  6. Restarts the Dolt server

Use --db to sync a single database, --dry-run to preview, or --force for force-push.
Use --gc to purge closed ephemeral beads (wisps, convoys) before pushing.

Examples:
  gt dolt sync                # Push all databases with remotes
  gt dolt sync --dry-run      # Preview what would be pushed
  gt dolt sync --db gastown   # Push only the gastown database
  gt dolt sync --force        # Force-push all databases
  gt dolt sync --gc           # Purge closed ephemeral beads, then push
  gt dolt sync --gc --dry-run # Preview purge + push without changes

```
gt dolt sync [flags]
```

### Options

```
      --db string   Sync a single database instead of all
      --dry-run     Preview what would be pushed without pushing
      --force       Force-push to remotes
      --gc          Purge closed ephemeral beads before push (requires bd purge)
  -h, --help        help for sync
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

