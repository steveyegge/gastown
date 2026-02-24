---
title: "DOCS/CLI/GT REFINERY ATTACH"
---

## gt refinery attach

Attach to refinery session

### Synopsis

Attach to a running Refinery's Claude session.

Allows interactive access to the Refinery agent for debugging
or manual intervention.

If rig is not specified, infers it from the current directory.

Examples:
  gt refinery attach greenplace
  gt refinery attach          # infer rig from cwd

```
gt refinery attach [rig] [flags]
```

### Options

```
      --agent string   Agent alias to run the Refinery with (overrides town default)
  -h, --help           help for attach
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

