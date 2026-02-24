---
title: "GT BEAD MOVE"
---

## gt bead move

Move a bead to a different repository

### Synopsis

Move a bead from one repository to another.

This creates a copy of the bead in the target repository (with the new prefix)
and closes the source bead with a reference to the new location.

The target prefix determines which repository receives the bead.
Common prefixes: gt- (gastown), bd- (beads), hq- (headquarters)

Examples:
  gt bead move gt-abc123 bd-     # Move gt-abc123 to beads repo as bd-*
  gt bead move hq-xyz bd-        # Move hq-xyz to beads repo
  gt bead move bd-123 gt-        # Move bd-123 to gastown repo

```
gt bead move <bead-id> <target-prefix> [flags]
```

### Options

```
  -n, --dry-run   Show what would be done
  -h, --help      help for move
```

### SEE ALSO

* [gt bead](../cli/gt_bead/)	 - Bead management utilities

