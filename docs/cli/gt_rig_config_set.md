---
title: "GT RIG CONFIG SET"
---

## gt rig config set

Set a configuration value

### Synopsis

Set a configuration value in the wisp layer (local, ephemeral).

Use --global to set in the bead layer (persistent, synced globally).
Use --block to explicitly block a key (prevents inheritance).

Examples:
  gt rig config set gastown status parked           # Wisp layer
  gt rig config set gastown status docked --global  # Bead layer
  gt rig config set gastown auto_restart --block    # Block inheritance

```
gt rig config set <rig> <key> [value] [flags]
```

### Options

```
      --block    Block inheritance for this key
      --global   Set in bead layer (persistent, synced)
  -h, --help     help for set
```

### SEE ALSO

* [gt rig config](../cli/gt_rig_config/)	 - View and manage rig configuration

