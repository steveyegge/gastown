---
title: "GT POLECAT PRUNE"
---

## gt polecat prune

Prune stale polecat branches (local and remote)

### Synopsis

Prune stale polecat branches in a rig.

Finds and deletes polecat branches that are no longer needed:
  - Branches fully merged to main
  - Branches whose remote tracking branch was deleted (post-merge cleanup)
  - Branches for polecats that no longer exist (orphaned)

Uses safe deletion (git branch -d) — only removes fully merged branches.
Also cleans up remote polecat branches that are fully merged.

Use --dry-run to preview what would be pruned.
Use --remote to also prune remote polecat branches on origin.

Examples:
  gt polecat prune greenplace
  gt polecat prune greenplace --dry-run
  gt polecat prune greenplace --remote

```
gt polecat prune <rig> [flags]
```

### Options

```
      --dry-run   Show what would be pruned without doing it
  -h, --help      help for prune
      --remote    Also prune remote polecat branches on origin
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

