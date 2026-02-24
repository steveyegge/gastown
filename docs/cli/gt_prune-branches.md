---
title: "DOCS/CLI/GT PRUNE-BRANCHES"
---

## gt prune-branches

Remove stale local polecat tracking branches

### Synopsis

Remove local branches that were created when tracking remote polecat branches.

When polecats push branches to origin, other clones create local tracking
branches via git fetch. After the remote branch is deleted (post-merge),
git fetch --prune removes the remote tracking ref but the local branch
persists indefinitely.

This command finds and removes local branches matching the pattern (default:
polecat/*) that are either:
  - Fully merged to the default branch (main)
  - Have no corresponding remote tracking branch (remote was deleted)

Safety: Uses git branch -d (not -D) so only fully-merged branches are deleted.
Never deletes the current branch or the default branch.

Examples:
  gt prune-branches              # Clean up stale polecat branches
  gt prune-branches --dry-run    # Show what would be deleted
  gt prune-branches --pattern "feature/*"  # Custom pattern

```
gt prune-branches [flags]
```

### Options

```
      --dry-run          Show what would be deleted without deleting
  -h, --help             help for prune-branches
      --pattern string   Branch name pattern to match (default "polecat/*")
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

