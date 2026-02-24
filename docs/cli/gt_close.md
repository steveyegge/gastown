---
title: "DOCS/CLI/GT CLOSE"
---

## gt close

Close one or more beads

### Synopsis

Close one or more beads (wrapper for 'bd close').

This is a convenience command that passes through to 'bd close' with
all arguments and flags preserved.

When an issue is closed, any convoys tracking it are checked for
completion. If all tracked issues in a convoy are closed, the convoy
is auto-closed.

Examples:
  gt close gt-abc              # Close bead gt-abc
  gt close gt-abc gt-def       # Close multiple beads
  gt close --reason "Done"     # Close with reason
  gt close --comment "Done"    # Same as --reason (alias)
  gt close --force             # Force close pinned beads

```
gt close [bead-id...] [flags]
```

### Options

```
  -h, --help   help for close
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

