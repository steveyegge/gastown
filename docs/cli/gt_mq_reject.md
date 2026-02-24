---
title: "DOCS/CLI/GT MQ REJECT"
---

## gt mq reject

Reject a merge request

### Synopsis

Manually reject a merge request.

This closes the MR with a 'rejected' status without merging.
The source issue is NOT closed (work is not done).

Examples:
  gt mq reject greenplace polecat/Nux/gp-xyz --reason "Does not meet requirements"
  gt mq reject greenplace mr-Nux-12345 --reason "Superseded by other work" --notify

```
gt mq reject <rig> <mr-id-or-branch> [flags]
```

### Options

```
  -h, --help            help for reject
      --notify          Send mail notification to worker
  -r, --reason string   Reason for rejection (required unless --stdin)
      --stdin           Read reason from stdin (avoids shell quoting issues)
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations

