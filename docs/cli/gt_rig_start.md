---
title: "GT RIG START"
---

## gt rig start

Start witness and refinery on patrol for one or more rigs

### Synopsis

Start the witness and refinery agents on patrol for one or more rigs.

This is similar to 'gt rig boot' but supports multiple rigs at once.
For each rig, it starts:
- The witness (if not already running)
- The refinery (if not already running)

Polecats are NOT started by this command - they are spawned
on demand when work is assigned.

Examples:
  gt rig start gastown
  gt rig start gastown beads
  gt rig start gastown beads myproject

```
gt rig start <rig>... [flags]
```

### Options

```
  -h, --help   help for start
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

