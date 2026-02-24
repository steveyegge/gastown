---
title: "DOCS/CLI/GT ENABLE"
---

## gt enable

Enable Gas Town system-wide

### Synopsis

Enable Gas Town for all agentic coding tools.

When enabled:
  - Shell hooks set GT_TOWN_ROOT and GT_RIG environment variables
  - Claude Code SessionStart hooks run 'gt prime' for context
  - Git repos are auto-registered as rigs (configurable)

Use 'gt disable' to turn off. Use 'gt status --global' to check state.

Environment overrides:
  GASTOWN_DISABLED=1  - Disable for current session only
  GASTOWN_ENABLED=1   - Enable for current session only

```
gt enable [flags]
```

### Options

```
  -h, --help   help for enable
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

