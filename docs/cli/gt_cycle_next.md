---
title: "GT CYCLE NEXT"
---

## gt cycle next

Switch to next session in group

### Synopsis

Switch to the next session in the current group.

This command is typically invoked via the C-b n keybinding. It automatically
detects whether you're in a town-level session (Mayor/Deacon) or a crew session
and cycles within the appropriate group.

Examples:
  gt cycle next
  gt cycle next --session gt-gastown-witness  # Explicit session context

```
gt cycle next [flags]
```

### Options

```
      --client string    Target client TTY (used by tmux binding, e.g. #{client_tty})
  -h, --help             help for next
      --session string   Override current session (used by tmux binding)
```

### SEE ALSO

* [gt cycle](../cli/gt_cycle/)	 - Cycle between sessions in the same group

