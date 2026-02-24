---
title: "GT POLECAT IDENTITY ADD"
---

## gt polecat identity add

Create an identity bead for a polecat

### Synopsis

Create an identity bead for a polecat in a rig.

If name is not provided, a name will be generated from the rig's name pool.

The identity bead tracks:
  - Role type (polecat)
  - Rig assignment
  - Agent state
  - Hook bead (current work)
  - Cleanup status

Example:
  gt polecat identity add gastown Toast
  gt polecat identity add gastown  # auto-generate name

```
gt polecat identity add <rig> [name] [flags]
```

### Options

```
  -h, --help   help for add
```

### SEE ALSO

* [gt polecat identity](../cli/gt_polecat_identity/)	 - Manage polecat identities

