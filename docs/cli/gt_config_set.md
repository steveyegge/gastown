---
title: "GT CONFIG SET"
---

## gt config set

Set a configuration value

### Synopsis

Set a town configuration value using dot-notation keys.

Supported keys:
  convoy.notify_on_complete   Push notification to Mayor session on convoy
                              completion (true/false, default: false)
  cli_theme                   CLI color scheme ("dark", "light", "auto")
  default_agent               Default agent preset name
  scheduler.max_polecats      Dispatch mode: -1 = direct (default), N > 0 = deferred
  scheduler.batch_size        Beads per heartbeat (default: 1)
  scheduler.spawn_delay       Delay between spawns (default: 0s)
  maintenance.window          Maintenance window start time in HH:MM (e.g., "03:00")
  maintenance.interval        How often: "daily", "weekly", "monthly", or duration
  maintenance.threshold       Commit count threshold (default: 1000)

  Lifecycle (Dolt data maintenance):
  lifecycle.reaper.enabled     Enable/disable wisp reaper (true/false)
  lifecycle.reaper.interval    Reaper check interval (default: 30m)
  lifecycle.reaper.delete_age  Delete closed wisps after this duration (default: 168h / 7d)
  lifecycle.compactor.enabled  Enable/disable compactor dog (true/false)
  lifecycle.compactor.interval Compactor check interval (default: 24h)
  lifecycle.compactor.threshold Commit count before compaction (default: 500)
  lifecycle.doctor.enabled     Enable/disable doctor dog (true/false)
  lifecycle.doctor.interval    Doctor check interval (default: 5m)
  lifecycle.backup.enabled     Enable/disable JSONL + Dolt backups (true/false)
  lifecycle.backup.interval    Backup interval (default: 15m)

Examples:
  gt config set convoy.notify_on_complete true
  gt config set cli_theme dark
  gt config set default_agent claude
  gt config set scheduler.max_polecats 5
  gt config set maintenance.window 03:00
  gt config set maintenance.interval daily
  gt config set lifecycle.reaper.delete_age 336h
  gt config set lifecycle.compactor.threshold 1000

```
gt config set <key> <value> [flags]
```

### Options

```
  -h, --help   help for set
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

