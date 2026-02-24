---
title: "GT QUOTA SCAN"
---

## gt quota scan

Detect rate-limited sessions

### Synopsis

Scan all Gas Town tmux sessions for rate-limit indicators.

Captures recent pane output from each session and checks for rate-limit
messages. Reports which sessions are blocked and which account they use.

Use --update to automatically update quota state with detected limits.

Examples:
  gt quota scan              # Report rate-limited sessions
  gt quota scan --update     # Report and update quota state
  gt quota scan --json       # JSON output

```
gt quota scan [flags]
```

### Options

```
  -h, --help     help for scan
      --json     Output as JSON
      --update   Update quota state with detected limits
```

### SEE ALSO

* [gt quota](../cli/gt_quota/)	 - Manage account quota rotation

