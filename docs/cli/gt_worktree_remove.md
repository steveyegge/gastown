---
title: "DOCS/CLI/GT WORKTREE REMOVE"
---

## gt worktree remove

Remove a cross-rig worktree

### Synopsis

Remove a git worktree created for cross-rig work.

This command removes a worktree that was previously created with 'gt worktree <rig>'.
It will refuse to remove a worktree with uncommitted changes unless --force is used.

Examples:
  gt worktree remove beads         # Remove beads worktree
  gt worktree remove beads --force # Force remove even with uncommitted changes

```
gt worktree remove <rig> [flags]
```

### Options

```
  -f, --force   Force remove even with uncommitted changes
  -h, --help    help for remove
```

### SEE ALSO

* [gt worktree](../cli/gt_worktree/)	 - Create worktree in another rig for cross-rig work

