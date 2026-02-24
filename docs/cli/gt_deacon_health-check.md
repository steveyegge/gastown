---
title: "DOCS/CLI/GT DEACON HEALTH-CHECK"
---

## gt deacon health-check

Send a health check ping to an agent and track response

### Synopsis

Send a HEALTH_CHECK nudge to an agent and wait for response.

This command is used by the Deacon during health rounds to detect stuck sessions.
It tracks consecutive failures and determines when force-kill is warranted.

The detection protocol:
1. Send HEALTH_CHECK nudge to the agent
2. Wait for agent to update their bead (configurable timeout, default 30s)
3. If no activity update, increment failure counter
4. After N consecutive failures (default 3), recommend force-kill

Exit codes:
  0 - Agent responded or is in cooldown (no action needed)
  1 - Error occurred
  2 - Agent should be force-killed (consecutive failures exceeded)

Examples:
  gt deacon health-check gastown/polecats/max
  gt deacon health-check gastown/witness --timeout=60s
  gt deacon health-check deacon --failures=5

```
gt deacon health-check <agent> [flags]
```

### Options

```
      --cooldown duration   Minimum time between force-kills of same agent (default 5m0s)
      --failures int        Number of consecutive failures before recommending force-kill (default 3)
  -h, --help                help for health-check
      --timeout duration    How long to wait for agent response (default 30s)
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

