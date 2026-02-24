---
title: "DOCS/CLI/GT DOG REMOVE"
---

## gt dog remove

Remove dogs from the kennel

### Synopsis

Remove one or more dogs from the kennel.

Removes all worktrees and the dog directory.
Use --force to remove even if dog is in working state.

Examples:
  gt dog remove alpha
  gt dog remove alpha bravo
  gt dog remove --all
  gt dog remove alpha --force

```
gt dog remove <name>... | --all [flags]
```

### Options

```
      --all     Remove all dogs
  -f, --force   Force removal even if working
  -h, --help    help for remove
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

