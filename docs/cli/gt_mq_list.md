---
title: "DOCS/CLI/GT MQ LIST"
---

## gt mq list

Show the merge queue

### Synopsis

Show the merge queue for a rig.

Lists all pending merge requests waiting to be processed.

Output format:
  ID          STATUS       PRIORITY  BRANCH                    WORKER  AGE
  gt-mr-001   ready        P0        polecat/Nux/gp-xyz        Nux     5m
  gt-mr-002   in_progress  P1        polecat/Toast/gt-abc      Toast   12m
  gt-mr-003   blocked      P1        polecat/Capable/gt-def    Capable 8m
              (waiting on gt-mr-001)

Examples:
  gt mq list greenplace
  gt mq list greenplace --ready
  gt mq list greenplace --status=open
  gt mq list greenplace --worker=Nux

```
gt mq list <rig> [flags]
```

### Options

```
      --epic string     Show MRs targeting integration/<epic>
  -h, --help            help for list
      --json            Output as JSON
      --ready           Show only ready-to-merge (no blockers)
      --status string   Filter by status (open, in_progress, closed)
      --verify          Verify branches exist in git (shows MISSING for deleted branches)
      --worker string   Filter by worker name
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations

