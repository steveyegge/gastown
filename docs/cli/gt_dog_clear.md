---
title: "GT DOG CLEAR"
---

## gt dog clear

Reset a stuck dog to idle state

### Synopsis

Reset a stuck dog to idle state.

Use this when a dog is stuck in "working" state but its session has died.
The Deacon uses this during patrol to clear dogs that have timed out.

By default, refuses to clear a dog if its tmux session still exists.
Use --force to clear even if the session is alive.

Examples:
  gt dog clear alpha           # Clear if session is dead
  gt dog clear alpha --force   # Force clear even if session exists

```
gt dog clear <name> [flags]
```

### Options

```
  -f, --force   Force clear even if session exists
  -h, --help    help for clear
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

