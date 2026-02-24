---
title: "GT REFINERY READY"
---

## gt refinery ready

List MRs ready for processing (unclaimed and unblocked)

### Synopsis

List merge requests ready for processing.

Shows MRs that are:
- Not currently claimed by any worker (or claim is stale)
- Not blocked by an open task (e.g., conflict resolution in progress)

This is the preferred command for finding work to process.

Use --all to see ALL open MRs (claimed, blocked, etc.) with raw data
including timestamps, assignees, and branch existence. Designed for
agent-side queue health analysis.

Examples:
  gt refinery ready
  gt refinery ready --json
  gt refinery ready --all --json

```
gt refinery ready [rig] [flags]
```

### Options

```
      --all    Show all open MRs (claimed, blocked, etc.) with raw data for queue health analysis
  -h, --help   help for ready
      --json   Output as JSON
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

