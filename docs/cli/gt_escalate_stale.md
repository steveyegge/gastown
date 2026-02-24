---
title: "DOCS/CLI/GT ESCALATE STALE"
---

## gt escalate stale

Re-escalate stale unacknowledged escalations

### Synopsis

Find and re-escalate escalations that haven't been acknowledged within the threshold.

When run without --dry-run, this command:
1. Finds escalations older than the stale threshold (default: 4h)
2. Bumps their severity: low→medium→high→critical
3. Re-routes them according to the new severity level
4. Sends mail to the new routing targets

Respects max_reescalations from config (default: 2) to prevent infinite escalation.

The threshold is configured in settings/escalation.json.

Examples:
  gt escalate stale              # Re-escalate stale escalations
  gt escalate stale --dry-run    # Show what would be done
  gt escalate stale --json       # JSON output of results

```
gt escalate stale [flags]
```

### Options

```
  -n, --dry-run   Show what would be re-escalated without acting
  -h, --help      help for stale
      --json      Output as JSON
```

### SEE ALSO

* [gt escalate](../cli/gt_escalate/)	 - Escalation system for critical issues

