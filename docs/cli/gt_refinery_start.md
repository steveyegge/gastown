---
title: "DOCS/CLI/GT REFINERY START"
---

## gt refinery start

Start the refinery

### Synopsis

Start the Refinery for a rig.

Launches the merge queue processor which monitors for polecat work branches
and merges them to the appropriate target branches.

If rig is not specified, infers it from the current directory.

Examples:
  gt refinery start greenplace
  gt refinery start greenplace --foreground
  gt refinery start              # infer rig from cwd

```
gt refinery start [rig] [flags]
```

### Options

```
      --agent string   Agent alias to run the Refinery with (overrides town default)
      --foreground     Run in foreground (default: background)
  -h, --help           help for start
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

