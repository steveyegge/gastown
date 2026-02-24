---
title: "GT REFINERY CLAIM"
---

## gt refinery claim

Claim an MR for processing

### Synopsis

Claim a merge request for processing by this refinery worker.

When running multiple refinery workers in parallel, each worker must claim
an MR before processing to prevent double-processing. Claims expire after
10 minutes if not processed (for crash recovery).

The worker ID is automatically determined from the GT_REFINERY_WORKER
environment variable, or defaults to "refinery-1".

Examples:
  gt refinery claim gt-abc123
  GT_REFINERY_WORKER=refinery-2 gt refinery claim gt-abc123

```
gt refinery claim <mr-id> [flags]
```

### Options

```
  -h, --help   help for claim
```

### SEE ALSO

* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)

