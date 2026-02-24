---
title: "DOCS/CLI/GT PLUGIN LIST"
---

## gt plugin list

List all discovered plugins

### Synopsis

List all plugins from town and rig plugin directories.

Plugins are discovered from:
  - ~/gt/plugins/ (town-level)
  - <rig>/plugins/ for each registered rig

When a plugin exists at both levels, the rig-level version takes precedence.

Examples:
  gt plugin list              # Human-readable output
  gt plugin list --json       # JSON output for scripting

```
gt plugin list [flags]
```

### Options

```
  -h, --help   help for list
      --json   Output as JSON
```

### SEE ALSO

* [gt plugin](../cli/gt_plugin/)	 - Plugin management

