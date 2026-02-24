---
title: "DOCS/CLI/GT DOLT MIGRATE"
---

## gt dolt migrate

Migrate existing dolt databases to centralized data directory

### Synopsis

Migrate existing dolt databases from .beads/dolt/ locations to the
centralized .dolt-data/ directory structure.

This command will:
1. Detect existing dolt databases in .beads/dolt/ directories
2. Move them to .dolt-data/<rigname>/
3. Remove the old empty directories

Use --dry-run to preview what would be moved (source/target paths and sizes)
without making any changes.

After migration, start the server with 'gt dolt start'.

```
gt dolt migrate [flags]
```

### Options

```
      --dry-run   Preview what would be migrated without making changes
  -h, --help      help for migrate
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

