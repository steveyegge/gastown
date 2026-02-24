---
title: "DOCS/CLI/GT ESCALATE LIST"
---

## gt escalate list

List open escalations

### Synopsis

List all open escalations.

Shows escalations that haven't been closed yet. Use --all to include
closed escalations.

Examples:
  gt escalate list              # Open escalations only
  gt escalate list --all        # Include closed
  gt escalate list --json       # JSON output

```
gt escalate list [flags]
```

### Options

```
      --all    Include closed escalations
  -h, --help   help for list
      --json   Output as JSON
```

### SEE ALSO

* [gt escalate](../cli/gt_escalate/)	 - Escalation system for critical issues

