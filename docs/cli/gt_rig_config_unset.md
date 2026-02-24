---
title: "DOCS/CLI/GT RIG CONFIG UNSET"
---

## gt rig config unset

Remove a configuration value from the wisp layer

### Synopsis

Remove a configuration value from the wisp layer.

This clears both regular values and blocked markers for the key.
Values set in the bead layer remain unchanged.

Example:
  gt rig config unset gastown status

```
gt rig config unset <rig> <key> [flags]
```

### Options

```
  -h, --help   help for unset
```

### SEE ALSO

* [gt rig config](../cli/gt_rig_config/)	 - View and manage rig configuration

