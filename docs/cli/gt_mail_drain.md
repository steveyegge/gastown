---
title: "GT MAIL DRAIN"
---

## gt mail drain

Bulk-archive stale protocol messages

### Synopsis

Bulk-archive stale protocol and lifecycle messages from an inbox.

Drains messages matching common protocol patterns that accumulate in
agent inboxes (especially witness). These are messages that have been
processed or are no longer actionable.

DRAINABLE MESSAGE TYPES:
  POLECAT_DONE       Polecat completion notifications
  POLECAT_STARTED    Polecat startup notifications
  LIFECYCLE:*        Lifecycle events (shutdown, etc.)
  MERGED             Merge confirmations
  MERGE_READY        Merge ready notifications
  MERGE_FAILED       Merge failure notifications
  SWARM_START        Swarm initiation messages

NON-DRAINABLE (preserved):
  HELP:*             Help requests (need human attention)
  HANDOFF            Session handoff context

By default, only archives protocol messages older than 30 minutes.
Use --max-age to change the threshold, or --all to drain regardless of age.

Examples:
  gt mail drain                              # Drain own inbox (30m default)
  gt mail drain --identity gastown/witness   # Drain witness inbox
  gt mail drain --max-age 1h                 # Only drain messages >1h old
  gt mail drain --all                        # Drain all protocol messages
  gt mail drain --dry-run                    # Preview what would be drained

```
gt mail drain [flags]
```

### Options

```
      --all               Drain all protocol messages regardless of age
  -n, --dry-run           Show what would be drained without archiving
  -h, --help              help for drain
      --identity string   Target inbox identity (e.g., gastown/witness)
      --max-age string    Only drain messages older than this duration (e.g., 30m, 1h, 2h) (default "30m")
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

