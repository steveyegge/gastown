---
title: "DOCS/CLI/GT CONFIG"
---

## gt config

Manage Gas Town configuration

### Synopsis

Manage Gas Town configuration settings.

This command allows you to view and modify configuration settings
for your Gas Town workspace, including agent aliases and defaults.

Commands:
  gt config agent list              List all agents (built-in and custom)
  gt config agent get <name>         Show agent configuration
  gt config agent set <name> <cmd>   Set custom agent command
  gt config agent remove <name>      Remove custom agent
  gt config default-agent [name]     Get or set default agent

```
gt config [flags]
```

### Options

```
  -h, --help   help for config
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt config agent](../cli/gt_config_agent/)	 - Manage agent configuration
* [gt config agent-email-domain](../cli/gt_config_agent-email-domain/)	 - Get or set agent email domain
* [gt config cost-tier](../cli/gt_config_cost-tier/)	 - Get or set cost optimization tier
* [gt config default-agent](../cli/gt_config_default-agent/)	 - Get or set default agent
* [gt config get](../cli/gt_config_get/)	 - Get a configuration value
* [gt config set](../cli/gt_config_set/)	 - Set a configuration value

