---
title: "DOCS/CLI/GT MQ"
---

## gt mq

Merge queue operations

### Synopsis

Manage merge requests and the merge queue for a rig.

Alias: 'gt mr' is equivalent to 'gt mq' (merge request vs merge queue).

The merge queue tracks work branches from polecats waiting to be merged.
Use these commands to view, submit, retry, and manage merge requests.

```
gt mq [flags]
```

### Options

```
  -h, --help   help for mq
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt mq integration](../cli/gt_mq_integration/)	 - Manage integration branches for epics
* [gt mq list](../cli/gt_mq_list/)	 - Show the merge queue
* [gt mq next](../cli/gt_mq_next/)	 - Show the highest-priority merge request
* [gt mq reject](../cli/gt_mq_reject/)	 - Reject a merge request
* [gt mq retry](../cli/gt_mq_retry/)	 - Retry a failed merge request
* [gt mq status](../cli/gt_mq_status/)	 - Show detailed merge request status
* [gt mq submit](../cli/gt_mq_submit/)	 - Submit current branch to the merge queue

