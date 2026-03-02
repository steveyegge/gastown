---
title: "GT MQ POST-MERGE"
---

## gt mq post-merge

Run post-merge cleanup (close MR, delete branch)

### Synopsis

Perform post-merge cleanup after a successful merge.

This command consolidates post-merge steps into a single atomic operation:
  1. Close the MR bead (status: merged)
  2. Close the source issue
  3. Delete the remote polecat branch (unless --skip-branch-delete)

Designed for use by the refinery formula after a successful merge to main.
The branch name is read from the MR bead, so no manual branch argument is needed.

Examples:
  gt mq post-merge gastown gt-mr-abc123
  gt mq post-merge gastown gt-mr-abc123 --skip-branch-delete

```
gt mq post-merge <rig> <mr-id> [flags]
```

### Options

```
  -h, --help                 help for post-merge
      --skip-branch-delete   Skip remote branch deletion
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations

