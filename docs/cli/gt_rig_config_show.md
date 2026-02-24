---
title: "DOCS/CLI/GT RIG CONFIG SHOW"
---

## gt rig config show

Show effective configuration for a rig

### Synopsis

Show the effective configuration for a rig.

By default, shows only the resolved values. Use --layers to see
which layer each value comes from.

Example output:
  gt rig config show gastown --layers
  Key                 Value        Source
  status              parked       wisp
  priority_adjustment 10           bead
  auto_restart        true         system
  max_polecats        4            town

```
gt rig config show <rig> [flags]
```

### Options

```
  -h, --help     help for show
      --layers   Show which layer each value comes from
```

### SEE ALSO

* [gt rig config](../cli/gt_rig_config/)	 - View and manage rig configuration

