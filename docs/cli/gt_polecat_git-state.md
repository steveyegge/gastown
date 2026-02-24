---
title: "GT POLECAT GIT-STATE"
---

## gt polecat git-state

Show git state for pre-kill verification

### Synopsis

Show git state for a polecat's worktree.

Used by the Witness for pre-kill verification to ensure no work is lost.
Returns whether the worktree is clean (safe to kill) or dirty (needs cleanup).

Checks:
  - Working tree: uncommitted changes
  - Unpushed commits: commits ahead of origin/main
  - Stashes: stashed changes

Examples:
  gt polecat git-state greenplace/Toast
  gt polecat git-state greenplace/Toast --json

```
gt polecat git-state <rig>/<polecat> [flags]
```

### Options

```
  -h, --help   help for git-state
      --json   Output as JSON
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

