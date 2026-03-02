---
title: "GT DOLT CLEANUP"
---

## gt dolt cleanup

Remove orphaned databases from .dolt-data/

### Synopsis

Detect and remove orphaned databases from the .dolt-data/ directory.

An orphaned database is one that exists in .dolt-data/ but is not referenced
by any rig's metadata.json. These are typically left over from partial setups,
renamed databases, or failed migrations.

Use --dry-run to preview what would be removed without making changes.

Examples:
  gt dolt cleanup             # Remove all orphaned databases
  gt dolt cleanup --dry-run   # Preview what would be removed

```
gt dolt cleanup [flags]
```

### Options

```
      --dry-run   Preview what would be removed without making changes
      --force     Remove databases even if they have user tables
  -h, --help      help for cleanup
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

