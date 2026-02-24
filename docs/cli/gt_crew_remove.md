---
title: "GT CREW REMOVE"
---

## gt crew remove

Remove crew workspace(s)

### Synopsis

Remove one or more crew workspaces from the rig.

Checks for uncommitted changes and running sessions before removing.
Use --force to skip checks and remove anyway.

The agent bead is CLOSED by default (preserves CV history). Use --purge
to DELETE the agent bead entirely (for accidental/test crew that should
leave no trace in the ledger).

--purge also:
  - Deletes the agent bead (not just closes it)
  - Unassigns any beads assigned to this crew member
  - Clears mail in the agent's inbox
  - Properly handles git worktrees (not just regular clones)

Examples:
  gt crew remove dave                       # Remove with safety checks
  gt crew remove dave emma fred             # Remove multiple
  gt crew remove beads/grip beads/fang      # Remove from specific rig
  gt crew remove dave --force               # Force remove (closes bead)
  gt crew remove test-crew --purge          # Obliterate (deletes bead)

```
gt crew remove <name...> [flags]
```

### Options

```
      --force        Force remove (skip safety checks)
  -h, --help         help for remove
      --purge        Obliterate: delete agent bead, unassign work, clear mail
      --rig string   Rig to use
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

