---
title: "GT MQ INTEGRATION STATUS"
---

## gt mq integration status

Show integration branch status for an epic

### Synopsis

Display the status of an integration branch.

Shows:
  - Integration branch name and creation date
  - Number of commits ahead of main
  - Merged MRs (closed, targeting integration branch)
  - Pending MRs (open, targeting integration branch)

Example:
  gt mq integration status gt-auth-epic

```
gt mq integration status <epic-id> [flags]
```

### Options

```
  -h, --help   help for status
      --json   Output as JSON
```

### SEE ALSO

* [gt mq integration](../cli/gt_mq_integration/)	 - Manage integration branches for epics

