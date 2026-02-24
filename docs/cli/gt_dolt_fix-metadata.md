---
title: "GT DOLT FIX-METADATA"
---

## gt dolt fix-metadata

Update metadata.json in all rig .beads directories

### Synopsis

Ensure all rig .beads/metadata.json files have correct Dolt server configuration.

This fixes the split-brain problem where bd falls back to local embedded databases
instead of connecting to the centralized Dolt server. It updates metadata.json with:
  - backend: "dolt"
  - dolt_mode: "server"
  - dolt_database: "<rigname>"

Safe to run multiple times (idempotent). Preserves any existing fields in metadata.json.

```
gt dolt fix-metadata [flags]
```

### Options

```
  -h, --help   help for fix-metadata
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

