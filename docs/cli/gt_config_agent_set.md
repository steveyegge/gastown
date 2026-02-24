---
title: "DOCS/CLI/GT CONFIG AGENT SET"
---

## gt config agent set

Set custom agent command

### Synopsis

Set a custom agent command in town settings.

This creates or updates a custom agent definition that overrides
or extends the built-in presets. The custom agent will be available
to all rigs in the town.

The command can include arguments. Use quotes if the command or
arguments contain spaces.

Examples:
  gt config agent set claude-glm \"claude-glm --model glm-4\"
  gt config agent set gemini-custom gemini --approval-mode yolo
  gt config agent set claude \"claude-glm\"  # Override built-in claude

```
gt config agent set <name> <command> [flags]
```

### Options

```
  -h, --help   help for set
```

### SEE ALSO

* [gt config agent](../cli/gt_config_agent/)	 - Manage agent configuration

