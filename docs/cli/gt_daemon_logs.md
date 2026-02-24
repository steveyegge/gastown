---
title: "DOCS/CLI/GT DAEMON LOGS"
---

## gt daemon logs

View daemon logs

### Synopsis

View the daemon log file.

Shows the most recent log entries from the daemon. Use -n to control
how many lines to display, or -f to follow the log in real time.

Examples:
  gt daemon logs             # Show last 50 lines
  gt daemon logs -n 100      # Show last 100 lines
  gt daemon logs -f           # Follow log output in real time

```
gt daemon logs [flags]
```

### Options

```
  -f, --follow      Follow log output
  -h, --help        help for logs
  -n, --lines int   Number of lines to show (default 50)
```

### SEE ALSO

* [gt daemon](../cli/gt_daemon/)	 - Manage the Gas Town daemon

