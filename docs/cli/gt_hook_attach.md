---
title: "GT HOOK ATTACH"
---

## gt hook attach

Attach work to a hook

### Synopsis

Attach a bead to your hook or another agent's hook.

With just a bead ID, attaches to your own hook (same as 'gt hook <bead-id>').
With a target, attaches to another agent's hook (for remote dispatch).

Examples:
  gt hook attach gt-abc                    # Attach to my hook
  gt hook attach gt-abc gastown/crew/max   # Attach to max's hook

```
gt hook attach <bead-id> [target] [flags]
```

### Options

```
  -f, --force   Replace existing incomplete hooked bead
  -h, --help    help for attach
```

### SEE ALSO

* [gt hook](../cli/gt_hook/)	 - Show or attach work on a hook

