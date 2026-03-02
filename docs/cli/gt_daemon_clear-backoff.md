---
title: "GT DAEMON CLEAR-BACKOFF"
---

## gt daemon clear-backoff

Clear crash loop backoff for an agent

### Synopsis

Clear the crash loop and restart backoff state for an agent.

When an agent crashes repeatedly, the daemon enters crash loop mode and
stops restarting it. Use this command to reset the crash loop counter so
the daemon will resume restarting the agent.

The agent name is the session identity (e.g., "deacon", "mayor").

Examples:
  gt daemon clear-backoff deacon   # Reset deacon crash loop

```
gt daemon clear-backoff <agent> [flags]
```

### Options

```
  -h, --help   help for clear-backoff
```

### SEE ALSO

* [gt daemon](../cli/gt_daemon/)	 - Manage the Gas Town daemon

