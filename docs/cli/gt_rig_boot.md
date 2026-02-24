---
title: "GT RIG BOOT"
---

## gt rig boot

Start witness and refinery for a rig

### Synopsis

Start the witness and refinery agents for a rig.

This is the inverse of 'gt rig shutdown'. It starts:
- The witness (if not already running)
- The refinery (if not already running)

Polecats are NOT started by this command - they are spawned
on demand when work is assigned.

Examples:
  gt rig boot greenplace

```
gt rig boot <rig> [flags]
```

### Options

```
  -h, --help   help for boot
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

