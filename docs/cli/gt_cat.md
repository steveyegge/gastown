---
title: "GT CAT"
---

## gt cat

Display bead content

### Synopsis

Display the content of a bead (issue, task, molecule, etc.).

This is a convenience wrapper around 'bd show' that integrates with gt.
Accepts any bead ID with a recognized prefix (gt-*, bd-*, hq-*, mol-*, etc.).

Examples:
  gt cat gt-abc123       # Show a gastown bead
  gt cat bd-abc123       # Show a beads bead
  gt cat hq-xyz789       # Show a town-level bead
  gt cat bd-abc --json   # Output as JSON

```
gt cat <bead-id> [flags]
```

### Options

```
  -h, --help   help for cat
      --json   Output as JSON
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

