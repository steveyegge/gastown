---
title: "GT DOLT INIT-RIG"
---

## gt dolt init-rig

Initialize a new rig database

### Synopsis

Initialize a new rig database in the Dolt data directory.

Each rig (e.g., gastown, beads) gets its own database that will be
served by the Dolt server. The rig name becomes the database name
when connecting via MySQL protocol.

Example:
  gt dolt init-rig gastown
  gt dolt init-rig beads

```
gt dolt init-rig <name> [flags]
```

### Options

```
  -h, --help   help for init-rig
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

