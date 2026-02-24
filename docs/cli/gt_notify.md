---
title: "DOCS/CLI/GT NOTIFY"
---

## gt notify

Set notification level

### Synopsis

Control the notification level for the current agent.

Notification levels:
  verbose  All notifications (mail, convoy events, status updates)
  normal   Important notifications only (default)
  muted    Silent/DND mode - batch notifications for later

Without arguments, shows the current notification level.

Examples:
  gt notify           # Show current level
  gt notify verbose   # Enable all notifications
  gt notify normal    # Default notification level
  gt notify muted     # Enable DND mode

Related: gt dnd - quick toggle for DND mode

```
gt notify [verbose|normal|muted] [flags]
```

### Options

```
  -h, --help   help for notify
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

