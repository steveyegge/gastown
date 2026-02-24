---
title: "DOCS/CLI/GT ESCALATE"
---

## gt escalate

Escalation system for critical issues

### Synopsis

Create and manage escalations for critical issues.

The escalation system provides severity-based routing for issues that need
human or mayor attention. Escalations are tracked as beads with gt:escalation label.

SEVERITY LEVELS:
  critical  (P0) Immediate attention required
  high      (P1) Urgent, needs attention soon
  medium    (P2) Standard escalation (default)
  low       (P3) Informational, can wait

WORKFLOW:
  1. Agent encounters blocking issue
  2. Runs: gt escalate "Description" --severity high --reason "details"
  3. Escalation is routed based on settings/escalation.json
  4. Recipient acknowledges with: gt escalate ack <id>
  5. After resolution: gt escalate close <id> --reason "fixed"

CONFIGURATION:
  Routing is configured in ~/gt/settings/escalation.json:
  - routes: Map severity to action lists (bead, mail:mayor, email:human, sms:human)
  - contacts: Human email/SMS for external notifications
  - stale_threshold: When unacked escalations are re-escalated (default: 4h)
  - max_reescalations: How many times to bump severity (default: 2)

Examples:
  gt escalate "Build failing" --severity critical --reason "CI blocked"
  gt escalate "Need API credentials" --severity high --source "plugin:rebuild-gt"
  gt escalate "Code review requested" --reason "PR #123 ready"
  gt escalate list                          # Show open escalations
  gt escalate ack hq-abc123                 # Acknowledge
  gt escalate close hq-abc123 --reason "Fixed in commit abc"
  gt escalate stale                         # Re-escalate stale escalations

```
gt escalate [description] [flags]
```

### Options

```
  -n, --dry-run           Show what would be done without executing
  -h, --help              help for escalate
      --json              Output as JSON
  -r, --reason string     Detailed reason for escalation
      --related string    Related bead ID (task, bug, etc.)
  -s, --severity string   Severity level: critical, high, medium, low (default "medium")
      --source string     Source identifier (e.g., plugin:rebuild-gt, patrol:deacon)
      --stdin             Read reason from stdin (avoids shell quoting issues)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt escalate ack](../cli/gt_escalate_ack/)	 - Acknowledge an escalation
* [gt escalate close](../cli/gt_escalate_close/)	 - Close a resolved escalation
* [gt escalate list](../cli/gt_escalate_list/)	 - List open escalations
* [gt escalate show](../cli/gt_escalate_show/)	 - Show details of an escalation
* [gt escalate stale](../cli/gt_escalate_stale/)	 - Re-escalate stale unacknowledged escalations

