---
title: "GT HOOKS OVERRIDE"
---

## gt hooks override

Edit overrides for a role or rig

### Synopsis

Edit hook overrides for a specific role or rig+role combination.

Valid targets:
  Role-level:  crew, witness, refinery, polecats, mayor, deacon
  Rig+role:    gastown/crew, beads/witness, sky/polecats, etc.

Overrides are merged on top of the base config during sync.
Hooks with the same matcher replace the base hook entirely.

Override files are stored in ~/.gt/hooks-overrides/<target>.json.

Examples:
  gt hooks override crew              # Edit crew role overrides
  gt hooks override gastown/crew      # Edit gastown rig crew overrides
  gt hooks override mayor             # Edit mayor overrides
  gt hooks override crew --show       # Print current override config

```
gt hooks override <target> [flags]
```

### Options

```
  -h, --help   help for override
      --show   Print current override config to stdout
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

