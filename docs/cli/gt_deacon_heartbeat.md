---
title: "GT DEACON HEARTBEAT"
---

## gt deacon heartbeat

Update the Deacon heartbeat

### Synopsis

Update the Deacon heartbeat file.

The heartbeat signals to the daemon that the Deacon is alive and working.
Call this at the start of each wake cycle to prevent daemon pokes.

Examples:
  gt deacon heartbeat                    # Touch heartbeat with timestamp
  gt deacon heartbeat "checking mayor"   # Touch with action description

```
gt deacon heartbeat [action] [flags]
```

### Options

```
  -h, --help   help for heartbeat
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

