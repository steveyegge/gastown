---
title: "GT POLECAT REMOVE"
---

## gt polecat remove

Remove polecats from a rig

### Synopsis

Remove one or more polecats from a rig.

Fails if session is running (stop first).
Warns if uncommitted changes exist.
Use --force to bypass checks.

Examples:
  gt polecat remove greenplace/Toast
  gt polecat remove greenplace/Toast greenplace/Furiosa
  gt polecat remove greenplace --all
  gt polecat remove greenplace --all --force

```
gt polecat remove <rig>/<polecat>... | <rig> --all [flags]
```

### Options

```
      --all     Remove all polecats in the rig
  -f, --force   Force removal, bypassing checks
  -h, --help    help for remove
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

