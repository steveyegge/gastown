---
title: "GT RIG RESTART"
---

## gt rig restart

Restart one or more rigs (stop then start)

### Synopsis

Restart the patrol agents (witness and refinery) for one or more rigs.

This is equivalent to 'gt rig stop' followed by 'gt rig start' for each rig.
Useful after polecats complete work and land their changes.

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to force immediate shutdown (prompts if uncommitted work).
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig restart gastown
  gt rig restart gastown beads
  gt rig restart --force gastown beads
  gt rig restart --nuclear gastown  # DANGER: loses uncommitted work

```
gt rig restart <rig>... [flags]
```

### Options

```
  -f, --force     Force immediate shutdown during restart (prompts if uncommitted work)
  -h, --help      help for restart
      --nuclear   DANGER: Bypass ALL safety checks (loses uncommitted work!)
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

