---
title: "DOCS/CLI/GT DEACON ZOMBIE-SCAN"
---

## gt deacon zombie-scan

Find and clean zombie Claude processes not in active tmux sessions

### Synopsis

Find and clean zombie Claude processes not in active tmux sessions.

Unlike cleanup-orphans (which uses TTY detection), zombie-scan uses tmux
verification: it checks if each Claude process is in an active tmux session
by comparing against actual pane PIDs.

A process is a zombie if:
- It's a Claude/codex process
- It's NOT the pane PID of any active tmux session
- It's NOT a child of any pane PID
- It's older than 60 seconds

This catches "ghost" processes that have a TTY (from a dead tmux session)
but are no longer part of any active Gas Town session.

Examples:
  gt deacon zombie-scan           # Find and kill zombies
  gt deacon zombie-scan --dry-run # Just list zombies, don't kill

```
gt deacon zombie-scan [flags]
```

### Options

```
      --dry-run   List zombies without killing them
  -h, --help      help for zombie-scan
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

