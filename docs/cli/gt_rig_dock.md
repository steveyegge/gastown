---
title: "GT RIG DOCK"
---

## gt rig dock

Dock a rig (global, persistent shutdown)

### Synopsis

Dock a rig to persistently disable it across all clones.

Docking a rig:
  - Stops the witness if running
  - Stops the refinery if running
  - Stops all polecat sessions if running
  - Sets status:docked label on the rig identity bead
  - Syncs via git so all clones see the docked status

This is a Level 2 (global/persistent) operation:
  - Affects all clones of this rig (via git sync)
  - Persists until explicitly undocked
  - The daemon respects this status and won't auto-restart agents

Use 'gt rig undock' to resume normal operation.

Examples:
  gt rig dock gastown
  gt rig dock beads

```
gt rig dock <rig> [flags]
```

### Options

```
  -h, --help   help for dock
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

