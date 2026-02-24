---
title: "DOCS/CLI/GT RIG STATUS"
---

## gt rig status

Show detailed status for a specific rig

### Synopsis

Show detailed status for a specific rig including all workers.

If no rig is specified, infers the rig from the current directory.

Displays:
- Rig information (name, path, beads prefix)
- Witness status (running/stopped, uptime)
- Refinery status (running/stopped, uptime, queue size)
- Polecats (name, state, assigned issue, session status)
- Crew members (name, branch, session status, git status)

Examples:
  gt rig status           # Infer rig from current directory
  gt rig status gastown
  gt rig status beads

```
gt rig status [rig] [flags]
```

### Options

```
  -h, --help   help for status
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

