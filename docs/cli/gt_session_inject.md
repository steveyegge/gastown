---
title: "DOCS/CLI/GT SESSION INJECT"
---

## gt session inject

Send message to session (prefer 'gt nudge')

### Synopsis

Send a message to a polecat session.

NOTE: For sending messages to Claude sessions, use 'gt nudge' instead.
It uses reliable delivery (literal mode + timing) that works correctly
with Claude Code's input handling.

This command is a low-level primitive for file-based injection or
cases where you need raw tmux send-keys behavior.

Examples:
  gt nudge greenplace/furiosa "Check your mail"     # Preferred
  gt session inject wyvern/Toast -f prompt.txt   # For file injection

```
gt session inject <rig>/<polecat> [flags]
```

### Options

```
  -f, --file string      File to read message from
  -h, --help             help for inject
  -m, --message string   Message to inject
```

### SEE ALSO

* [gt session](../cli/gt_session/)	 - Manage polecat sessions

