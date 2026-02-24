---
title: "GT RIG UNDOCK"
---

## gt rig undock

Undock a rig (remove global docked status)

### Synopsis

Undock a rig to remove the persistent docked status.

Undocking a rig:
  - Removes the status:docked label from the rig identity bead
  - Syncs via git so all clones see the undocked status
  - Allows the daemon to auto-restart agents
  - Does NOT automatically start agents (use 'gt rig start' for that)

Examples:
  gt rig undock gastown
  gt rig undock beads

```
gt rig undock <rig> [flags]
```

### Options

```
  -h, --help   help for undock
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

