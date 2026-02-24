---
title: "DOCS/CLI/GT CONFIG GET"
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

Examples:
  gt config get convoy.notify_on_complete
  gt config get cli_theme
  gt config get scheduler.max_polecats

```
gt config get <key> [flags]
```

### Options

```
  -h, --help   help for get
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

