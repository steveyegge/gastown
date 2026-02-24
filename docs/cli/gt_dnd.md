---
title: "GT DND"
---

## gt dnd

Toggle Do Not Disturb mode for notifications

### Synopsis

Control notification level for the current agent.

Do Not Disturb (DND) mode mutes non-critical notifications,
allowing you to focus on work without interruption.

Subcommands:
  on      Enable DND mode (mute notifications)
  off     Disable DND mode (resume normal notifications)
  status  Show current notification level

Without arguments, toggles DND mode.

Related: gt notify - for fine-grained notification level control

Examples:
  gt dnd            # Toggle DND on/off
  gt dnd on         # Enable DND (mute notifications)
  gt dnd off        # Disable DND (resume notifications)
  gt dnd status     # Show current notification level

```
gt dnd [on|off|status] [flags]
```

### Options

```
  -h, --help   help for dnd
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

