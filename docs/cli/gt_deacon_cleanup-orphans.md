---
title: "GT DEACON CLEANUP-ORPHANS"
---

## gt deacon cleanup-orphans

Clean up orphaned claude subagent processes

### Synopsis

Clean up orphaned claude subagent processes.

Claude Code's Task tool spawns subagent processes that sometimes don't clean up
properly after completion. These accumulate and consume significant memory.

Detection is based on TTY column: processes with TTY "?" have no controlling
terminal. Legitimate claude instances in terminals have a TTY like "pts/0".

This is safe because:
- Processes in terminals (your personal sessions) have a TTY - won't be touched
- Only kills processes that have no controlling terminal
- These orphans are children of the tmux server with no TTY

Example:
  gt deacon cleanup-orphans

```
gt deacon cleanup-orphans [flags]
```

### Options

```
  -h, --help   help for cleanup-orphans
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

