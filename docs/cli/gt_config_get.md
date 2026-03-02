---
title: "GT CONFIG GET"
---

## gt config get

Get a configuration value

### Synopsis

Get a town configuration value using dot-notation keys.

Supported keys:
  convoy.notify_on_complete   Push notification to Mayor session on convoy
                              completion (true/false, default: false)
  cli_theme                   CLI color scheme
  default_agent               Default agent preset name
  scheduler.max_polecats      Dispatch mode (-1 = direct, N > 0 = deferred)
  scheduler.batch_size        Beads per heartbeat
  scheduler.spawn_delay       Delay between spawns
  maintenance.window          Maintenance window start time (HH:MM)
  maintenance.interval        How often: daily, weekly, monthly, or duration
  maintenance.threshold       Commit count threshold

  Lifecycle (Dolt data maintenance):
  lifecycle.reaper.enabled     Wisp reaper enabled (true/false)
  lifecycle.reaper.interval    Reaper check interval
  lifecycle.reaper.delete_age  Duration before closed wisps are deleted
  lifecycle.compactor.enabled  Compactor dog enabled (true/false)
  lifecycle.compactor.interval Compactor check interval
  lifecycle.compactor.threshold Commit count threshold for compaction
  lifecycle.doctor.enabled     Doctor dog enabled (true/false)
  lifecycle.doctor.interval    Doctor check interval
  lifecycle.backup.enabled     JSONL + Dolt backups enabled (true/false)
  lifecycle.backup.interval    Backup interval

Examples:
  gt config get convoy.notify_on_complete
  gt config get cli_theme
  gt config get maintenance.window
  gt config get lifecycle.reaper.delete_age

```
gt config get <key> [flags]
```

### Options

```
  -h, --help   help for get
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

