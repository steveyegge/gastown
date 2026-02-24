---
title: "DOCS/CLI/GT CREW STATUS"
---

## gt crew status

Show detailed workspace status

### Synopsis

Show detailed status for crew workspace(s).

Displays session state, git status, branch info, and mail inbox status.
If no name given, shows status for all crew workers.

Examples:
  gt crew status                  # Status of all crew workers
  gt crew status dave             # Status of specific worker
  gt crew status --json           # JSON output

```
gt crew status [<name>] [flags]
```

### Options

```
  -h, --help         help for status
      --json         Output as JSON
      --rig string   Filter by rig name
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

