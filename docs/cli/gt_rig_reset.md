---
title: "GT RIG RESET"
---

## gt rig reset

Reset rig state (handoff content, mail, stale issues)

### Synopsis

Reset various rig state.

By default, resets all resettable state. Use flags to reset specific items.

Examples:
  gt rig reset              # Reset all state
  gt rig reset --handoff    # Clear handoff content only
  gt rig reset --mail       # Clear stale mail messages only
  gt rig reset --stale      # Reset orphaned in_progress issues
  gt rig reset --stale --dry-run  # Preview what would be reset

```
gt rig reset [flags]
```

### Options

```
      --dry-run       Show what would be reset without making changes
      --handoff       Clear handoff content
  -h, --help          help for reset
      --mail          Clear stale mail messages
      --role string   Role to reset (default: auto-detect from cwd)
      --stale         Reset orphaned in_progress issues (no active session)
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

