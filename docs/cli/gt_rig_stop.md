---
title: "DOCS/CLI/GT RIG STOP"
---

## gt rig stop

Stop one or more rigs (shutdown semantics)

### Synopsis

Stop all agents in one or more rigs.

This command is similar to 'gt rig shutdown' but supports multiple rigs.
For each rig, it gracefully shuts down:
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
  gt rig stop gastown
  gt rig stop gastown beads
  gt rig stop --force gastown beads
  gt rig stop --nuclear gastown  # DANGER: loses uncommitted work

```
gt rig stop <rig>... [flags]
```

### Options

```
  -f, --force     Force immediate shutdown (prompts if uncommitted work)
  -h, --help      help for stop
      --nuclear   DANGER: Bypass ALL safety checks (loses uncommitted work!)
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

