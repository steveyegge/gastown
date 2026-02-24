---
title: "DOCS/CLI/GT DEACON HEALTH-STATE"
---

## gt deacon health-state

Show health check state for all monitored agents

### Synopsis

Display the current health check state including:
- Consecutive failure counts
- Last ping and response times
- Force-kill history and cooldowns

This helps the Deacon understand which agents may need attention.

```
gt deacon health-state [flags]
```

### Options

```
  -h, --help   help for health-state
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

