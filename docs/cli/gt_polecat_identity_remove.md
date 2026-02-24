---
title: "GT POLECAT IDENTITY REMOVE"
---

## gt polecat identity remove

Remove a polecat identity

### Synopsis

Remove a polecat identity bead.

Safety checks:
  - No active tmux session
  - No work on hook (unless using --force)
  - Warns if CV exists

Use --force to bypass safety checks.

Example:
  gt polecat identity remove gastown Toast
  gt polecat identity remove gastown Toast --force

```
gt polecat identity remove <rig> <name> [flags]
```

### Options

```
  -f, --force   Force removal, bypassing safety checks
  -h, --help    help for remove
```

### SEE ALSO

* [gt polecat identity](../cli/gt_polecat_identity/)	 - Manage polecat identities

