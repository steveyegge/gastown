---
title: "GT PATROL DIGEST"
---

## gt patrol digest

Aggregate patrol cycle digests into a daily summary bead

### Synopsis

Aggregate ephemeral patrol cycle digests into a permanent daily summary.

This command is intended to be run by Deacon patrol (daily) or manually.
It queries patrol digests for a target date, creates a single aggregate
"Patrol Report YYYY-MM-DD" bead, then deletes the source digests.

The resulting digest bead is permanent (synced via git) and provides
an audit trail without per-cycle ephemeral pollution.

Examples:
  gt patrol digest --yesterday   # Digest yesterday's patrols (for daily patrol)
  gt patrol digest --date 2026-01-15
  gt patrol digest --yesterday --dry-run

```
gt patrol digest [flags]
```

### Options

```
      --date string   Digest patrol cycles for specific date (YYYY-MM-DD)
      --dry-run       Preview what would be created without creating
  -h, --help          help for digest
  -v, --verbose       Verbose output
      --yesterday     Digest yesterday's patrol cycles
```

### SEE ALSO

* [gt patrol](../cli/gt_patrol/)	 - Patrol digest management

