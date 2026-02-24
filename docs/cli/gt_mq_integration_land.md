---
title: "GT MQ INTEGRATION LAND"
---

## gt mq integration land

Merge integration branch to main

### Synopsis

Merge an epic's integration branch to main.

Lands all work for an epic by merging its integration branch to main
as a single atomic merge commit.

Actions:
  1. Verify all MRs targeting integration/<epic> are merged
  2. Verify integration branch exists
  3. Merge integration/<epic> to main (--no-ff)
  4. Run tests on main
  5. Push to origin
  6. Delete integration branch
  7. Update epic status

Options:
  --force       Land even if some MRs still open
  --skip-tests  Skip test run
  --dry-run     Preview only, make no changes

Examples:
  gt mq integration land gt-auth-epic
  gt mq integration land gt-auth-epic --dry-run
  gt mq integration land gt-auth-epic --force --skip-tests

```
gt mq integration land <epic-id> [flags]
```

### Options

```
      --dry-run      Preview only, make no changes
      --force        Land even if some MRs still open
  -h, --help         help for land
      --skip-tests   Skip test run
```

### SEE ALSO

* [gt mq integration](../cli/gt_mq_integration/)	 - Manage integration branches for epics

