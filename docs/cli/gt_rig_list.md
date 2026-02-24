---
title: "DOCS/CLI/GT RIG LIST"
---

## gt rig list

List all rigs in the workspace

### Synopsis

List all rigs registered in the Gas Town workspace.

For each rig, displays:
  - Rig name and operational state (OPERATIONAL, PARKED, DOCKED)
  - Witness status (running/stopped)
  - Refinery status (running/stopped)
  - Number of polecats and crew members

Examples:
  gt rig list          # List all rigs with status
  gt rig list --json   # Output as JSON for scripting

```
gt rig list [flags]
```

### Options

```
  -h, --help   help for list
      --json   Output as JSON
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

