---
title: "DOCS/CLI/GT REFINERY RESTART"
---

## gt refinery restart

Restart the refinery

### Synopsis

Restart the Refinery for a rig.

Stops the current session (if running) and starts a fresh one.
If rig is not specified, infers it from the current directory.

Examples:
  gt refinery restart greenplace
  gt refinery restart          # infer rig from cwd

```
gt refinery restart [rig] [flags]
```

### Options

```
      --agent string   Agent alias to run the Refinery with (overrides town default)
  -h, --help           help for restart
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

