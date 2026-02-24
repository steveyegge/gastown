---
title: "GT WITNESS ATTACH"
---

## gt witness attach

Attach to witness session

### Synopsis

Attach to the Witness tmux session for a rig.

Attaches the current terminal to the witness's tmux session.
Detach with Ctrl-B D.

If the witness is not running, this will start it first.
If rig is not specified, infers it from the current directory.

Examples:
  gt witness attach greenplace
  gt witness attach          # infer rig from cwd

```
gt witness attach [rig] [flags]
```

### Options

```
  -h, --help   help for attach
```

### SEE ALSO

* [gt witness](../cli/gt_witness/)	 - Manage the Witness (per-rig polecat health monitor)

