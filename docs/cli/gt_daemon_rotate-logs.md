---
title: "GT DAEMON ROTATE-LOGS"
---

## gt daemon rotate-logs

Rotate daemon log files

### Synopsis

Rotate all daemon-managed log files.

Uses copytruncate for Dolt server logs (safe for processes with open fds).
daemon.log uses automatic lumberjack rotation and is skipped.

By default, only rotates logs exceeding 100MB. Use --force to rotate all.

Examples:
  gt daemon rotate-logs           # Rotate logs > 100MB
  gt daemon rotate-logs --force   # Rotate all logs regardless of size

```
gt daemon rotate-logs [flags]
```

### Options

```
      --force   Rotate all logs regardless of size
  -h, --help    help for rotate-logs
```

### SEE ALSO

* [gt daemon](../cli/gt_daemon/)	 - Manage the Gas Town daemon

