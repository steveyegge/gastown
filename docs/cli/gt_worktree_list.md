---
title: "GT WORKTREE LIST"
---

## gt worktree list

List all cross-rig worktrees owned by current crew member

### Synopsis

List all git worktrees created for cross-rig work.

This command scans all rigs in the workspace and finds worktrees
that belong to the current crew member. Each worktree is shown with
its git status summary.

Example output:
  Cross-rig worktrees for gastown/crew/joe:

    beads     ~/gt/beads/crew/gastown-joe/     (clean)
    mayor     ~/gt/mayor/crew/gastown-joe/     (2 uncommitted)

```
gt worktree list [flags]
```

### Options

```
  -h, --help   help for list
```

### SEE ALSO

* [gt worktree](../cli/gt_worktree/)	 - Create worktree in another rig for cross-rig work

