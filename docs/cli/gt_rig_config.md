---
title: "GT RIG CONFIG"
---

## gt rig config

View and manage rig configuration

### Synopsis

View and manage rig configuration across property layers.

Configuration is looked up through multiple layers:
1. Wisp layer (transient, local) - .beads-wisp/config/
2. Bead layer (persistent, synced) - rig identity bead labels
3. Town defaults - ~/gt/settings/config.json
4. System defaults - compiled-in fallbacks

Most properties use override semantics (first non-nil wins).
Integer properties like priority_adjustment use stacking semantics (values add).

```
gt rig config [flags]
```

### Options

```
  -h, --help   help for config
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace
* [gt rig config set](../cli/gt_rig_config_set/)	 - Set a configuration value
* [gt rig config show](../cli/gt_rig_config_show/)	 - Show effective configuration for a rig
* [gt rig config unset](../cli/gt_rig_config_unset/)	 - Remove a configuration value from the wisp layer

