---
title: "DOCS/CLI/GT RIG SETTINGS SET"
---

## gt rig settings set

Set a settings value

### Synopsis

Set a settings value using dot notation for nested keys.

The value type is automatically inferred:
- "true"/"false" → boolean
- Numbers → number
- Valid JSON → parsed as JSON
- Otherwise → string

If the settings file doesn't exist, it will be created with a valid scaffold.

Examples:
  gt rig settings set gastown agent claude
  gt rig settings set gastown role_agents.witness gemini
  gt rig settings set gastown merge_queue.max_concurrent 5
  gt rig settings set gastown theme.background_color "#000000"

```
gt rig settings set <rig> <key-path> <value> [flags]
```

### Options

```
  -h, --help   help for set
```

### SEE ALSO

* [gt rig settings](../cli/gt_rig_settings/)	 - View and manage rig settings

