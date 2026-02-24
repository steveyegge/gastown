---
title: "GT CREW REFRESH"
---

## gt crew refresh

Context cycling with mail-to-self handoff

### Synopsis

Cycle a crew workspace session with handoff.

Sends a handoff mail to the workspace's own inbox, then restarts the session.
The new session reads the handoff mail and resumes work.

Examples:
  gt crew refresh dave                           # Refresh with auto-generated handoff
  gt crew refresh dave -m "Working on gt-123"    # Add custom message

```
gt crew refresh <name> [flags]
```

### Options

```
  -h, --help             help for refresh
  -m, --message string   Custom handoff message
      --rig string       Rig to use
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

