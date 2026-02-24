---
title: "GT START CREW"
---

## gt start crew

Start a crew workspace (creates if needed)

### Synopsis

Start a crew workspace, creating it if it doesn't exist.

This is a convenience command that combines 'gt crew add' and 'gt crew at --detached'.
The crew session starts in the background with Claude running and ready.

The name can include the rig in slash format (e.g., greenplace/joe).
If not specified, the rig is inferred from the current directory.

Examples:
  gt start crew joe                    # Start joe in current rig
  gt start crew greenplace/joe            # Start joe in gastown rig
  gt start crew joe --rig beads        # Start joe in beads rig

```
gt start crew <name> [flags]
```

### Options

```
      --account string   Claude Code account handle to use
      --agent string     Agent alias to run crew worker with (overrides rig/town default)
  -h, --help             help for crew
      --rig string       Rig to use
```

### SEE ALSO

* [gt start](../cli/gt_start/)	 - Start Gas Town or a crew workspace

