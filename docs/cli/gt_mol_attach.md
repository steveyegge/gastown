---
title: "DOCS/CLI/GT MOL ATTACH"
---

## gt mol attach

Attach a molecule to a pinned bead

### Synopsis

Attach a molecule to a pinned/handoff bead.

This records which molecule an agent is currently working on. The attachment
is stored in the pinned bead's description and visible via 'bd show'.

When called with a single argument from an agent working directory, the
pinned bead ID is auto-detected from the current agent's hook.

Examples:
  gt molecule attach gt-abc mol-xyz  # Explicit pinned bead
  gt molecule attach mol-xyz         # Auto-detect from cwd

```
gt mol attach [pinned-bead-id] <molecule-id> [flags]
```

### Options

```
  -h, --help   help for attach
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

