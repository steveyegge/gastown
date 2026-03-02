---
title: "GT POLECAT POOL-INIT"
---

## gt polecat pool-init

Initialize a persistent polecat pool for a rig

### Synopsis

Initialize a persistent polecat pool for a rig.

Creates N polecats with identities and worktrees in IDLE state,
ready for immediate work assignment via gt sling.

Pool size is determined by (in priority order):
  1. --size flag
  2. polecat_pool_size in rig config.json
  3. Default: 4

Polecat names come from:
  1. polecat_names in rig config.json (if specified)
  2. The rig's name pool theme (default: mad-max)

Existing polecats are preserved — only new ones are created
to reach the target pool size.

Examples:
  gt polecat pool-init gastown
  gt polecat pool-init gastown --size 6
  gt polecat pool-init gastown --dry-run

```
gt polecat pool-init <rig> [flags]
```

### Options

```
      --dry-run    Show what would be created without doing it
  -h, --help       help for pool-init
      --size int   Pool size (overrides rig config)
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

