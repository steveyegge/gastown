---
title: "GT ROLE DEF"
---

## gt role def

Display role definition (session, health, env config)

### Synopsis

Display the effective role definition after all overrides are applied.

Role configuration is layered:
  1. Built-in defaults (embedded in binary)
  2. Town-level overrides (~/.gt/roles/<role>.toml)
  3. Rig-level overrides (<rig>/roles/<role>.toml)

Examples:
  gt role def witness    # Show witness role definition
  gt role def crew       # Show crew role definition

```
gt role def <role> [flags]
```

### Options

```
  -h, --help   help for def
```

### SEE ALSO

* [gt role](../cli/gt_role/)	 - Show or manage agent role

