---
title: "DOCS/CLI/GT WORKTREE"
---

## gt worktree

Create worktree in another rig for cross-rig work

### Synopsis

Create a git worktree in another rig for cross-rig work.

This command is for crew workers who need to work on another rig's codebase
while maintaining their identity. It creates a worktree in the target rig's
crew/ directory with a name that identifies your source rig and identity.

The worktree is created at: ~/gt/<target-rig>/crew/<source-rig>-<name>/

For example, if you're gastown/crew/joe and run 'gt worktree beads':
- Creates worktree at ~/gt/beads/crew/gastown-joe/
- The worktree checks out main branch
- Your identity (BD_ACTOR, GT_ROLE) remains gastown/crew/joe

Use --no-cd to just print the path without printing shell commands.

Examples:
  gt worktree beads         # Create worktree in beads rig
  gt worktree gastown       # Create worktree in gastown rig (from another rig)
  gt worktree beads --no-cd # Just print the path

```
gt worktree <rig> [flags]
```

### Options

```
  -h, --help    help for worktree
      --no-cd   Just print path (don't print cd command)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt worktree list](../cli/gt_worktree_list/)	 - List all cross-rig worktrees owned by current crew member
* [gt worktree remove](../cli/gt_worktree_remove/)	 - Remove a cross-rig worktree

