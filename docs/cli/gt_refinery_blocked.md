---
title: "DOCS/CLI/GT REFINERY BLOCKED"
---

## gt refinery blocked

List MRs blocked by open tasks

### Synopsis

List merge requests blocked by open tasks.

Shows MRs waiting for conflict resolution or other blocking tasks to complete.
When the blocking task closes, the MR will appear in 'ready'.

Examples:
  gt refinery blocked
  gt refinery blocked --json

```
gt refinery blocked [rig] [flags]
```

### Options

```
  -h, --help   help for blocked
      --json   Output as JSON
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

