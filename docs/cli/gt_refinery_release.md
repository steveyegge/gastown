---
title: "DOCS/CLI/GT REFINERY RELEASE"
---

## gt refinery release

Release a claimed MR back to the queue

### Synopsis

Release a claimed merge request back to the queue.

Called when processing fails and the MR should be retried by another worker.
This clears the claim so other workers can pick up the MR.

Examples:
  gt refinery release gt-abc123

```
gt refinery release <mr-id> [flags]
```

### Options

```
  -h, --help   help for release
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

