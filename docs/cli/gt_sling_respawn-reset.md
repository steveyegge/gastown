---
title: "GT SLING RESPAWN-RESET"
---

## gt sling respawn-reset

Reset the respawn counter for a bead

### Synopsis

Reset the per-bead respawn counter so it can be slung again.

When a bead hits the respawn limit (3 attempts), gt sling blocks further
dispatches to prevent spawn storms. After investigating the root cause,
use this command to allow re-dispatch.

```
gt sling respawn-reset <bead-id> [flags]
```

### Options

```
  -h, --help   help for respawn-reset
```

### SEE ALSO

* [gt sling](../cli/gt_sling/)	 - Assign work to an agent (THE unified work dispatch command)

