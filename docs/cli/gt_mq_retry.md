---
title: "DOCS/CLI/GT MQ RETRY"
---

## gt mq retry

Retry a failed merge request

### Synopsis

Retry a failed merge request.

Resets a failed MR so it can be processed again by the refinery.
The MR must be in a failed state (open with an error).

Examples:
  gt mq retry greenplace gp-mr-abc123
  gt mq retry greenplace gp-mr-abc123 --now

```
gt mq retry <rig> <mr-id> [flags]
```

### Options

```
  -h, --help   help for retry
      --now    Immediately process instead of waiting for refinery loop
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations

