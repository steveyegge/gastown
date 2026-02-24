---
title: "DOCS/CLI/GT POLECAT IDENTITY RENAME"
---

## gt polecat identity rename

Rename a polecat identity (preserves CV)

### Synopsis

Rename a polecat identity bead, preserving CV history.

The rename:
  1. Creates a new identity bead with the new name
  2. Copies CV history links to the new bead
  3. Closes the old bead with a reference to the new one

Safety checks:
  - Old identity must exist
  - New name must not already exist
  - Polecat session must not be running

Example:
  gt polecat identity rename gastown Toast Imperator

```
gt polecat identity rename <rig> <old-name> <new-name> [flags]
```

### Options

```
  -h, --help   help for rename
```

### SEE ALSO

* [gt polecat identity](../cli/gt_polecat_identity/)	 - Manage polecat identities

