---
title: "DOCS/CLI/GT PATROL NEW"
---

## gt patrol new

Create a new patrol wisp with config variables

### Synopsis

Create a new patrol wisp for the current role, injecting rig config
variables so the formula has correct settings baked in.

Role is auto-detected from GT_ROLE (set by the daemon). Use --role to override.

For refinery patrols, MQ config variables (run_tests, test_command,
target_branch, etc.) are read from the rig's config.json and settings/config.json and
passed as --var args to the wisp.

Examples:
  gt patrol new                  # Auto-detect role, create patrol
  gt patrol new --role refinery  # Explicitly create refinery patrol

```
gt patrol new [flags]
```

### Options

```
  -h, --help          help for new
      --role string   Role override (deacon, witness, refinery)
```

### SEE ALSO

* [gt patrol](../cli/gt_patrol/)	 - Patrol digest management

