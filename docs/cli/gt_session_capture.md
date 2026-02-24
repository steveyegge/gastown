---
title: "GT SESSION CAPTURE"
---

## gt session capture

Capture recent session output

### Synopsis

Capture recent output from a polecat session.

Returns the last N lines of terminal output. Useful for checking progress.

Examples:
  gt session capture wyvern/Toast        # Last 100 lines (default)
  gt session capture wyvern/Toast 50     # Last 50 lines
  gt session capture wyvern/Toast -n 50  # Same as above

```
gt session capture <rig>/<polecat> [count] [flags]
```

### Options

```
  -h, --help        help for capture
  -n, --lines int   Number of lines to capture (default 100)
```

### SEE ALSO

* [gt session](../cli/gt_session/)	 - Manage polecat sessions

