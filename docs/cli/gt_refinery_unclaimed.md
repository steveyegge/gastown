---
title: "DOCS/CLI/GT REFINERY UNCLAIMED"
---

## gt refinery unclaimed

List unclaimed MRs available for processing

### Synopsis

List merge requests that are available for claiming.

Shows MRs that are not currently claimed by any worker, or have stale
claims (worker may have crashed). Useful for parallel refinery workers
to find work.

Examples:
  gt refinery unclaimed
  gt refinery unclaimed --json

```
gt refinery unclaimed [rig] [flags]
```

### Options

```
  -h, --help   help for unclaimed
      --json   Output as JSON
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

