---
title: "GT COMPACT"
---

## gt compact

Compact expired wisps (TTL-based cleanup)

### Synopsis

Apply TTL-based compaction policy to ephemeral wisps.

For non-closed wisps past TTL: promotes to permanent beads (something is stuck).
For closed wisps past TTL: deletes them (Dolt AS OF preserves history).
Wisps with comments or keep labels are always promoted.

TTLs by wisp type:
  heartbeat, ping:              6h
  patrol, gc_report:            24h
  recovery, error, escalation:  7d
  default (untyped):            24h

Examples:
  gt compact              # Run compaction
  gt compact --dry-run    # Preview what would happen
  gt compact --verbose    # Show each wisp decision
  gt compact --json       # Machine-readable output

```
gt compact [flags]
```

### Options

```
      --dry-run      Preview compaction without making changes
  -h, --help         help for compact
      --json         Output results as JSON
      --rig string   Compact a specific rig (default: current rig)
  -v, --verbose      Show each wisp decision
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt compact report](../cli/gt_compact_report/)	 - Generate and send compaction digest report

