---
title: "GT RIG REMOVE"
---

## gt rig remove

Remove a rig from the registry (does not delete files)

### Synopsis

Remove a rig from the Gas Town registry.

This only removes the rig entry from mayor/rigs.json and cleans up
the beads route. The rig's files on disk are NOT deleted.

If the rig has running tmux sessions (witness, refinery, polecats, crew),
you must shut them down first with 'gt rig shutdown' or use --force to
kill them automatically.

To fully remove a rig, delete the directory manually after unregistering.

Examples:
  gt rig remove myproject                    # Unregister (fails if sessions running)
  gt rig remove myproject --force            # Kill sessions then unregister
  gt rig remove myproject && rm -rf myproject # Unregister and delete files

```
gt rig remove <name> [flags]
```

### Options

```
  -f, --force   Kill running tmux sessions before removing (may lose uncommitted work)
  -h, --help    help for remove
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

