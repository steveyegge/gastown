---
title: "DOCS/CLI/GT ROLE HOME"
---

## gt role home

Show home directory for a role

### Synopsis

Show the canonical home directory for a role.

If no role is specified, shows the home for the current role.

Examples:
  gt role home           # Home for current role
  gt role home mayor     # Home for mayor
  gt role home witness   # Home for witness (requires --rig)

```
gt role home [ROLE] [flags]
```

### Options

```
  -h, --help             help for home
      --polecat string   Polecat/crew member name
      --rig string       Rig name (required for rig-specific roles)
```

### SEE ALSO

* [gt role](../cli/gt_role/)	 - Show or manage agent role

