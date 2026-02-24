---
title: "GT POLECAT GC"
---

## gt polecat gc

Garbage collect stale polecat branches

### Synopsis

Garbage collect stale polecat branches in a rig.

Polecats use unique timestamped branches (polecat/<name>-<timestamp>) to
prevent drift issues. Over time, these branches accumulate when stale
polecats are repaired.

This command removes orphaned branches:
  - Branches for polecats that no longer exist
  - Old timestamped branches (keeps only the current one per polecat)

Examples:
  gt polecat gc greenplace
  gt polecat gc greenplace --dry-run

```
gt polecat gc <rig> [flags]
```

### Options

```
      --dry-run   Show what would be deleted without deleting
  -h, --help      help for gc
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

