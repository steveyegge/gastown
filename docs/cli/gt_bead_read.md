---
title: "DOCS/CLI/GT BEAD READ"
---

## gt bead read

Show details of a bead (alias for 'show')

### Synopsis

Displays the full details of a bead by ID.

This is an alias for 'gt bead show'. All bd show flags are supported.

Examples:
  gt bead read gt-abc123          # Show a gastown issue
  gt bead read hq-xyz789          # Show a town-level bead
  gt bead read bd-def456          # Show a beads issue
  gt bead read gt-abc123 --json   # Output as JSON

```
gt bead read <bead-id> [flags]
```

### Options

```
  -h, --help   help for read
```

### SEE ALSO

* [gt bead](../cli/gt_bead/)	 - Bead management utilities

