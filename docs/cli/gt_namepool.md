---
title: "DOCS/CLI/GT NAMEPOOL"
---

## gt namepool

Manage polecat name pools

### Synopsis

Manage themed name pools for polecats in Gas Town.

By default, polecats get themed names from the Mad Max universe
(furiosa, nux, slit, etc.). You can change the theme or add custom names.

Examples:
  gt namepool              # Show current pool status
  gt namepool --list       # List available themes
  gt namepool themes       # Show theme names
  gt namepool set minerals # Set theme to 'minerals'
  gt namepool add ember    # Add custom name to pool
  gt namepool reset        # Reset pool state

```
gt namepool [flags]
```

### Options

```
  -h, --help   help for namepool
  -l, --list   List available themes
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt namepool add](../cli/gt_namepool_add/)	 - Add a custom name to the pool
* [gt namepool reset](../cli/gt_namepool_reset/)	 - Reset the pool state (release all names)
* [gt namepool set](../cli/gt_namepool_set/)	 - Set the namepool theme for this rig
* [gt namepool themes](../cli/gt_namepool_themes/)	 - List available themes and their names

