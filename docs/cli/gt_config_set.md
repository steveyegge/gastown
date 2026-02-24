---
title: "DOCS/CLI/GT CONFIG SET"
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

Examples:
  gt config set convoy.notify_on_complete true
  gt config set cli_theme dark
  gt config set default_agent claude
  gt config set scheduler.max_polecats 5
  gt config set scheduler.max_polecats -1

```
gt config set <key> <value> [flags]
```

### Options

```
  -h, --help   help for set
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

