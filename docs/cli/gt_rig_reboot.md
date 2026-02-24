---
title: "GT RIG REBOOT"
---

## gt rig reboot

Restart witness and refinery for a rig

### Synopsis

Restart the patrol agents (witness and refinery) for a rig.

This is equivalent to 'gt rig shutdown' followed by 'gt rig boot'.
Useful after polecats complete work and land their changes.

Examples:
  gt rig reboot greenplace
  gt rig reboot beads --force

```
gt rig reboot <rig> [flags]
```

### Options

```
  -f, --force     Force immediate shutdown during reboot (prompts if uncommitted work)
  -h, --help      help for reboot
      --nuclear   DANGER: Bypass ALL safety checks during reboot (loses uncommitted work!)
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

