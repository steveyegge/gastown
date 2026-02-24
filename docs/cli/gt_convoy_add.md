---
title: "GT CONVOY ADD"
---

## gt convoy add

Add issues to an existing convoy

### Synopsis

Add issues to an existing convoy.

If the convoy is closed, it will be automatically reopened.

Examples:
  gt convoy add hq-cv-abc gt-new-issue
  gt convoy add hq-cv-abc gt-issue1 gt-issue2 gt-issue3
  gt convoy add hq-cv-abc --notify mayor/  # Reopen and notify

```
gt convoy add <convoy-id> <issue-id> [issue-id...] [flags]
```

### Options

```
  -h, --help   help for add
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

