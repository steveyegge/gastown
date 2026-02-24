---
title: "GT CONVOY CLOSE"
---

## gt convoy close

Close a convoy

### Synopsis

Close a convoy, optionally with a reason.

By default, verifies that all tracked issues are closed before allowing the
close. Use --force to close regardless of tracked issue status.

The close is idempotent - closing an already-closed convoy is a no-op.

Examples:
  gt convoy close hq-cv-abc                           # Close (all items must be done)
  gt convoy close hq-cv-abc --force                   # Force close abandoned convoy
  gt convoy close hq-cv-abc --reason="no longer needed" --force
  gt convoy close hq-cv-xyz --notify mayor/

```
gt convoy close <convoy-id> [flags]
```

### Options

```
  -f, --force           Close even if tracked issues are still open
  -h, --help            help for close
      --notify string   Agent to notify on close (e.g., mayor/)
      --reason string   Reason for closing the convoy
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

