---
title: "DOCS/CLI/GT COMPACT REPORT"
---

## gt compact report

Generate and send compaction digest report

### Synopsis

Generate a compaction digest and send it to deacon/ (cc mayor/).

The daily digest shows per-category breakdown of deleted, promoted, and active
wisps, plus any promotions with reasons and detected anomalies.

The weekly rollup (--weekly) aggregates the past 7 days of compaction event
beads and sends trend data to mayor/.

Examples:
  gt compact report              # Run compaction + send daily digest
  gt compact report --dry-run    # Preview the report without sending
  gt compact report --weekly     # Send weekly rollup to mayor/
  gt compact report --json       # Output report as JSON

```
gt compact report [flags]
```

### Options

```
      --date string   Report for specific date (YYYY-MM-DD); default: today
      --dry-run       Preview report without sending
  -h, --help          help for report
      --json          Output report as JSON
  -v, --verbose       Verbose output
      --weekly        Generate weekly rollup instead of daily digest
```

### SEE ALSO

* [gt compact](../cli/gt_compact/)	 - Compact expired wisps (TTL-based cleanup)

