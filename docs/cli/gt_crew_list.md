---
title: "GT CREW LIST"
---

## gt crew list

List crew workspaces with status

### Synopsis

List all crew workspaces in a rig with their status.

Shows git branch, session state, and git status for each workspace.

Examples:
  gt crew list                    # List in current rig
  gt crew list greenplace         # List in specific rig (positional)
  gt crew list --rig greenplace   # List in specific rig (flag)
  gt crew list --all              # List in all rigs
  gt crew list --json             # JSON output

```
gt crew list [rig] [flags]
```

### Options

```
      --all          List crew workspaces in all rigs
  -h, --help         help for list
      --json         Output as JSON
      --rig string   Filter by rig name
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

