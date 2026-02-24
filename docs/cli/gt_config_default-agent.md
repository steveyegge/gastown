---
title: "DOCS/CLI/GT CONFIG DEFAULT-AGENT"
---

## gt config default-agent

Get or set default agent

### Synopsis

Get or set the default agent for the town.

With no arguments, shows the current default agent.
With an argument, sets the default agent to the specified name.

The default agent is used when a rig doesn't specify its own agent
setting. Can be a built-in preset (claude, gemini, codex) or a
custom agent name.

Examples:
  gt config default-agent           # Show current default
  gt config default-agent claude    # Set to claude
  gt config default-agent gemini    # Set to gemini
  gt config default-agent my-custom # Set to custom agent

```
gt config default-agent [name] [flags]
```

### Options

```
  -h, --help   help for default-agent
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

