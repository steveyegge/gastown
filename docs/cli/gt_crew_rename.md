---
title: "DOCS/CLI/GT CREW RENAME"
---

## gt crew rename

Rename a crew workspace

### Synopsis

Rename a crew workspace.

Kills any running session, renames the directory, and updates state.
The new session will use the new name (gt-<rig>-crew-<new-name>).

Examples:
  gt crew rename dave david       # Rename dave to david
  gt crew rename madmax max       # Rename madmax to max

```
gt crew rename <old-name> <new-name> [flags]
```

### Options

```
  -h, --help         help for rename
      --rig string   Rig to use
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

