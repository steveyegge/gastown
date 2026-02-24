---
title: "GT DOLT MIGRATE-WISPS"
---

## gt dolt migrate-wisps

Migrate agent beads from issues to wisps table

### Synopsis

Create the wisps table infrastructure and migrate existing agent beads.

This command:
1. Creates the wisps table (dolt_ignored, same schema as issues)
2. Creates auxiliary tables (wisp_labels, wisp_comments, wisp_events, wisp_dependencies)
3. Copies agent beads (issue_type='agent') from issues to wisps
4. Copies associated labels, comments, events, and dependencies
5. Closes the originals in the issues table

Idempotent — safe to run multiple times. Use --dry-run to preview.

After migration, 'bd mol wisp list' will work and agent lifecycle
(spawn, sling, work, done, nuke, respawn) uses the wisps table.

```
gt dolt migrate-wisps [flags]
```

### Options

```
      --db string   Target database (default: auto-detect from rig)
      --dry-run     Preview what would be migrated without making changes
  -h, --help        help for migrate-wisps
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

