---
title: "GT KRC CONFIG"
---

## gt krc config

View or modify TTL configuration

### Synopsis

View or modify the KRC TTL configuration.

Without arguments, shows the current configuration.

Subcommands:
  set <pattern> <ttl>   Set TTL for event type pattern
  reset                 Reset to default configuration

Examples:
  gt krc config                     # Show current config
  gt krc config set patrol_* 12h    # Set patrol TTL to 12 hours
  gt krc config set default 3d      # Set default TTL to 3 days
  gt krc config reset               # Reset to defaults

```
gt krc config [subcommand] [flags]
```

### Options

```
  -h, --help   help for config
```

### SEE ALSO

* [gt krc](../cli/gt_krc/)	 - Key Record Chronicle - manage ephemeral data TTLs
* [gt krc config reset](../cli/gt_krc_config_reset/)	 - Reset TTL configuration to defaults
* [gt krc config set](../cli/gt_krc_config_set/)	 - Set TTL for an event type pattern

