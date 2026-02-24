---
title: "GT MAIL CLEAR"
---

## gt mail clear

Clear all messages from an inbox

### Synopsis

Clear (delete) all messages from an inbox.

SYNTAX:
  gt mail clear              # Clear your own inbox
  gt mail clear <target>     # Clear another agent's inbox

BEHAVIOR:
1. List all messages in the target inbox
2. Delete each message
3. Print count of deleted messages

Use case: Town quiescence - reset all inboxes across workers efficiently.

Examples:
  gt mail clear                      # Clear your inbox
  gt mail clear gastown/polecats/joe # Clear joe's inbox
  gt mail clear mayor/               # Clear mayor's inbox

```
gt mail clear [target] [flags]
```

### Options

```
      --all    Clear all messages (default behavior)
  -h, --help   help for clear
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

