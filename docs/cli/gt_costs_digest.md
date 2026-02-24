---
title: "DOCS/CLI/GT COSTS DIGEST"
---

## gt costs digest

Aggregate session cost log entries into a daily digest bead

### Synopsis

Aggregate session cost log entries into a permanent daily digest.

This command is intended to be run by Deacon patrol (daily) or manually.
It reads entries from ~/.gt/costs.jsonl for a target date, creates a single
aggregate "Cost Report YYYY-MM-DD" bead, then removes the source entries.

The resulting digest bead is permanent (synced via git) and provides
an audit trail without log-in-database pollution.

Examples:
  gt costs digest --yesterday   # Digest yesterday's costs (default for patrol)
  gt costs digest --date 2026-01-07  # Digest a specific date
  gt costs digest --yesterday --dry-run  # Preview without changes

```
gt costs digest [flags]
```

### Options

```
      --date string   Digest a specific date (YYYY-MM-DD)
      --dry-run       Preview what would be done without making changes
  -h, --help          help for digest
      --yesterday     Digest yesterday's costs (default for patrol)
```

### SEE ALSO

* [gt costs](../cli/gt_costs/)	 - Show costs for running Claude sessions

