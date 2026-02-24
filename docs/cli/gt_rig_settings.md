---
title: "DOCS/CLI/GT RIG SETTINGS"
---

## gt rig settings

View and manage rig settings

### Synopsis

View and manage rig settings (settings/config.json).

Rig settings control behavioral configuration for a rig:
- Agent selection and overrides
- Merge queue settings
- Theme configuration
- Namepool settings
- Crew startup settings
- Workflow settings

Settings are stored in settings/config.json within each rig directory.
Use dot notation to access nested keys (e.g., role_agents.witness).

```
gt rig settings [flags]
```

### Options

```
  -h, --help   help for settings
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace
* [gt rig settings set](../cli/gt_rig_settings_set/)	 - Set a settings value
* [gt rig settings show](../cli/gt_rig_settings_show/)	 - Display all settings
* [gt rig settings unset](../cli/gt_rig_settings_unset/)	 - Remove a settings value

