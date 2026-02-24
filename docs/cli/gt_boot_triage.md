---
title: "DOCS/CLI/GT BOOT TRIAGE"
---

## gt boot triage

Run triage directly (degraded mode)

### Synopsis

Run Boot's triage logic directly without Claude.

This is for degraded mode operation when tmux is unavailable.
It performs basic observation and takes conservative action:
  - If Deacon is not running: start it
  - If Deacon appears stuck: attempt restart
  - Otherwise: do nothing

Use --degraded flag when running in degraded mode.

```
gt boot triage [flags]
```

### Options

```
      --degraded   Run in degraded mode (no tmux)
  -h, --help       help for triage
```

### SEE ALSO

* [gt boot](../cli/gt_boot/)	 - Manage Boot (Deacon watchdog)

