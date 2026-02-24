---
title: "DOCS/CLI/GT PLUGIN HISTORY"
---

## gt plugin history

Show plugin execution history

### Synopsis

Show recent execution history for a plugin.

Queries ephemeral beads (wisps) that record plugin runs.

Examples:
  gt plugin history rebuild-gt
  gt plugin history rebuild-gt --json
  gt plugin history rebuild-gt --limit 20

```
gt plugin history <name> [flags]
```

### Options

```
  -h, --help        help for history
      --json        Output as JSON
      --limit int   Maximum number of runs to show (default 10)
```

### SEE ALSO

* [gt plugin](../cli/gt_plugin/)	 - Plugin management

