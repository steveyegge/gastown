---
title: "GT MQ NEXT"
---

## gt mq next

Show the highest-priority merge request

### Synopsis

Show the next merge request to process based on priority score.

The priority scoring function considers:
  - Convoy age: Older convoys get higher priority (starvation prevention)
  - Issue priority: P0 > P1 > P2 > P3 > P4
  - Retry count: MRs that fail repeatedly get deprioritized
  - MR age: FIFO tiebreaker for same priority/convoy

Use --strategy=fifo for first-in-first-out ordering instead.

Examples:
  gt mq next gastown                    # Show highest-priority MR
  gt mq next gastown --strategy=fifo    # Show oldest MR instead
  gt mq next gastown --quiet            # Just print the MR ID
  gt mq next gastown --json             # Output as JSON

```
gt mq next <rig> [flags]
```

### Options

```
  -h, --help              help for next
      --json              Output as JSON
  -q, --quiet             Just print the MR ID
      --strategy string   Ordering strategy: 'priority' or 'fifo' (default "priority")
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations

