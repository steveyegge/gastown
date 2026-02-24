---
title: "DOCS/CLI/GT CALLBACKS PROCESS"
---

## gt callbacks process

Process pending callbacks

### Synopsis

Process all pending callbacks in the Mayor's inbox.

Reads unread messages from the Mayor's inbox and handles each based on
its type:

  POLECAT_DONE       - Log completion, update stats
  MERGE_COMPLETED    - Notify worker, close source issue
  MERGE_REJECTED     - Notify worker of rejection reason
  HELP:              - Route to human or handle if possible
  ESCALATION:        - Log and route to human
  SLING_REQUEST:     - Spawn polecat for the work

Note: Witnesses and Refineries handle routine operations autonomously.
They only send escalations for genuine problems, not status reports.

Unknown message types are logged but left unprocessed.

```
gt callbacks process [flags]
```

### Options

```
      --dry-run   Show what would be processed without taking action
  -h, --help      help for process
  -v, --verbose   Show detailed processing info
```

### SEE ALSO

* [gt callbacks](../cli/gt_callbacks/)	 - Handle agent callbacks

