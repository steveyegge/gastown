---
title: "DOCS/CLI/GT ESCALATE CLOSE"
---

## gt escalate close

Close a resolved escalation

### Synopsis

Close an escalation after the issue is resolved.

Records who closed it and the resolution reason.

Examples:
  gt escalate close hq-abc123 --reason "Fixed in commit abc"
  gt escalate close hq-abc123 --reason "Not reproducible"

```
gt escalate close <escalation-id> [flags]
```

### Options

```
  -h, --help            help for close
      --reason string   Resolution reason
```

### SEE ALSO

* [gt escalate](../cli/gt_escalate/)	 - Escalation system for critical issues

