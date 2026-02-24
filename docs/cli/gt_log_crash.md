---
title: "DOCS/CLI/GT LOG CRASH"
---

## gt log crash

Record a crash event (called by tmux pane-died hook)

### Synopsis

Record a crash event to the town log.

This command is called automatically by tmux when a pane exits unexpectedly.
It's not typically run manually.

The exit code determines if this was a crash or expected exit:
  - Exit code 0: Expected exit (logged as 'done' if no other done was recorded)
  - Exit code non-zero: Crash (logged as 'crash')

Examples:
  gt log crash --agent greenplace/Toast --session gt-greenplace-Toast --exit-code 1

```
gt log crash [flags]
```

### Options

```
      --agent string     Agent ID (e.g., greenplace/Toast)
      --exit-code int    Exit code from pane (default -1)
  -h, --help             help for crash
      --session string   Tmux session name
```

### SEE ALSO

* [gt log](../cli/gt_log/)	 - View town activity log

