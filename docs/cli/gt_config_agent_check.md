---
title: "GT CONFIG AGENT CHECK"
---

## gt config agent check

Verify agent configs will auto-start correctly

### Synopsis

Check all agent definitions and role assignments for configuration issues.

Verifies that:
  - TUI agents (pir, pi) have prompt_mode=arg so beacons are delivered
  - All role_agents entries resolve to valid agent definitions
  - Agent commands are installed and on PATH
  - Hook files exist at expected locations

This catches the most common misconfiguration: agents that spawn but sit
idle because prompt_mode is "none" or unset, so the startup beacon never
reaches the agent as a CLI argument.

Examples:
  gt config agent check            # Check all agents and roles
  gt config agent check --verbose  # Show passing checks too

```
gt config agent check [flags]
```

### Options

```
  -h, --help      help for check
  -v, --verbose   Show passing checks too
```

### SEE ALSO

* [gt config agent](../cli/gt_config_agent/)	 - Manage agent configuration

