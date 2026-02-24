---
title: "GT RIG SETTINGS UNSET"
---

## gt rig settings unset

Remove a settings value

### Synopsis

Remove a settings value using dot notation for nested keys.

This removes the key from the settings file. For nested keys, only the
specified key is removed (parent objects remain if they have other keys).

Examples:
  gt rig settings unset gastown agent
  gt rig settings unset gastown role_agents.witness

```
gt rig settings unset <rig> <key-path> [flags]
```

### Options

```
  -h, --help   help for unset
```

### SEE ALSO

* [gt rig settings](../cli/gt_rig_settings/)	 - View and manage rig settings

