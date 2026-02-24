---
title: "DOCS/CLI/GT ESCALATE ACK"
---

## gt escalate ack

Acknowledge an escalation

### Synopsis

Acknowledge an escalation to indicate you're working on it.

Adds an "acked" label and records who acknowledged and when.
This stops the stale escalation warnings.

Examples:
  gt escalate ack hq-abc123

```
gt escalate ack <escalation-id> [flags]
```

### Options

```
  -h, --help   help for ack
```

### SEE ALSO

* [gt escalate](../cli/gt_escalate/)	 - Escalation system for critical issues

