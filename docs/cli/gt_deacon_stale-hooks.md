---
title: "GT DEACON STALE-HOOKS"
---

## gt deacon stale-hooks

Find and unhook stale hooked beads

### Synopsis

Find beads stuck in 'hooked' status and unhook them if the agent is gone.

Beads can get stuck in 'hooked' status when agents die or abandon work.
This command finds hooked beads older than the threshold (default: 1 hour),
checks if the assignee agent is still alive, and unhooks them if not.

Examples:
  gt deacon stale-hooks                 # Find and unhook stale beads
  gt deacon stale-hooks --dry-run       # Preview what would be unhooked
  gt deacon stale-hooks --max-age=30m   # Use 30 minute threshold

```
gt deacon stale-hooks [flags]
```

### Options

```
      --dry-run            Preview what would be unhooked without making changes
  -h, --help               help for stale-hooks
      --max-age duration   Maximum age before a hooked bead is considered stale (default 1h0m0s)
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

