---
title: "GT RIG SHUTDOWN"
---

## gt rig shutdown

Gracefully stop all rig agents

### Synopsis

Stop all agents in a rig.

This command gracefully shuts down:
- All polecat sessions
- The refinery (if running)
- The witness (if running)

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to force immediate shutdown (prompts if uncommitted work).
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig shutdown greenplace
  gt rig shutdown greenplace --force
  gt rig shutdown greenplace --nuclear  # DANGER: loses uncommitted work

```
gt rig shutdown <rig> [flags]
```

### Options

```
  -f, --force     Force immediate shutdown (prompts if uncommitted work)
  -h, --help      help for shutdown
      --nuclear   DANGER: Bypass ALL safety checks (loses uncommitted work!)
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

