---
title: "GT SESSION CHECK"
---

## gt session check

Check session health for polecats

### Synopsis

Check if polecat tmux sessions are alive and healthy.

This command validates that:
1. Polecats with work-on-hook have running tmux sessions
2. Sessions are responsive

Use this for manual health checks or debugging session issues.

Examples:
  gt session check              # Check all rigs
  gt session check greenplace      # Check specific rig

```
gt session check [rig] [flags]
```

### Options

```
  -h, --help   help for check
```

### SEE ALSO

* [gt session](../cli/gt_session/)	 - Manage polecat sessions

