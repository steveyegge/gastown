---
title: "GT CYCLE PREV"
---

## gt cycle prev

Switch to previous session in group

### Synopsis

Switch to the previous session in the current group.

This command is typically invoked via the C-b p keybinding. It automatically
detects whether you're in a town-level session (Mayor/Deacon) or a crew session
and cycles within the appropriate group.

Examples:
  gt cycle prev
  gt cycle prev --session gt-gastown-witness  # Explicit session context

```
gt cycle prev [flags]
```

### Options

```
      --client string    Target client TTY (used by tmux binding, e.g. #{client_tty})
  -h, --help             help for prev
      --session string   Override current session (used by tmux binding)
```

### SEE ALSO

* [gt cycle](../cli/gt_cycle/)	 - Cycle between sessions in the same group

