---
title: "DOCS/CLI/GT DOLT INIT"
---

## gt dolt init

Initialize and repair Dolt workspace configuration

### Synopsis

Verify and repair the Dolt workspace configuration.

This command scans all rig metadata.json files for Dolt server configuration
and ensures the referenced databases actually exist. It fixes the broken state
where metadata.json says backend=dolt but the database is missing from .dolt-data/.

For each broken workspace, it will:
  1. Check if local .beads/dolt/ data exists and migrate it
  2. Otherwise, create a fresh database in .dolt-data/

This is safe to run multiple times (idempotent). It will not modify workspaces
that are already healthy.

```
gt dolt init [flags]
```

### Options

```
  -h, --help   help for init
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

