---
title: "DOCS/CLI/GT CONVOY CHECK"
---

## gt convoy check

Check and auto-close completed convoys

### Synopsis

Check convoys and auto-close any where all tracked issues are complete.

Without arguments, checks all open convoys. With a convoy ID, checks only that convoy.

This handles cross-rig convoy completion: convoys in town beads tracking issues
in rig beads won't auto-close via bd close alone. This command bridges that gap.

Can be run manually or by deacon patrol to ensure convoys close promptly.

Examples:
  gt convoy check              # Check all open convoys
  gt convoy check hq-cv-abc    # Check specific convoy
  gt convoy check --dry-run    # Preview what would close without acting

```
gt convoy check [convoy-id] [flags]
```

### Options

```
      --dry-run   Preview what would close without acting
  -h, --help      help for check
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

