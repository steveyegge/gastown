---
title: "DOCS/CLI/GT PLUGIN"
---

## gt plugin

Plugin management

### Synopsis

Manage plugins that run during Deacon patrol cycles.

Plugins are periodic automation tasks defined by plugin.md files with TOML frontmatter.

PLUGIN LOCATIONS:
  ~/gt/plugins/           Town-level plugins (universal, apply everywhere)
  <rig>/plugins/          Rig-level plugins (project-specific)

GATE TYPES:
  cooldown    Run if enough time has passed (e.g., 1h)
  cron        Run on a schedule (e.g., "0 9 * * *")
  condition   Run if a check command returns exit 0
  event       Run on events (e.g., startup)
  manual      Never auto-run, trigger explicitly

Examples:
  gt plugin list                    # List all discovered plugins
  gt plugin show <name>             # Show plugin details
  gt plugin list --json             # JSON output

```
gt plugin [flags]
```

### Options

```
  -h, --help   help for plugin
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt plugin history](../cli/gt_plugin_history/)	 - Show plugin execution history
* [gt plugin list](../cli/gt_plugin_list/)	 - List all discovered plugins
* [gt plugin run](../cli/gt_plugin_run/)	 - Manually trigger plugin execution
* [gt plugin show](../cli/gt_plugin_show/)	 - Show plugin details

