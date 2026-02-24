---
title: "GT RELEASE"
---

## gt release

Release stuck in_progress issues back to pending

### Synopsis

Release one or more in_progress issues back to open/pending status.

This is used to recover stuck steps when a worker dies mid-task.
The issue is moved to "open" status and the assignee is cleared,
allowing another worker to claim and complete it.

Examples:
  gt release gt-abc           # Release single issue
  gt release gt-abc gt-def    # Release multiple issues
  gt release gt-abc -r "worker died"  # Release with reason

This implements nondeterministic idempotence - work can be safely
retried by releasing and reclaiming stuck steps.

```
gt release <issue-id>... [flags]
```

### Options

```
  -h, --help            help for release
  -r, --reason string   Reason for releasing (added as note)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

