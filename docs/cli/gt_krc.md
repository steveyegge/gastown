---
title: "GT KRC"
---

## gt krc

Key Record Chronicle - manage ephemeral data TTLs

### Synopsis

Key Record Chronicle (KRC) manages TTL-based lifecycle for Level 0 ephemeral data.

Per DOLT-STORAGE-DESIGN-V3.md, Level 0 includes patrol heartbeats, status checks,
and other operational data that decays in forensic value over days.

KRC provides:
  - Configurable TTLs per event type
  - Auto-pruning of expired events
  - Statistics on ephemeral data lifecycle

Examples:
  gt krc stats              # Show event statistics
  gt krc prune              # Remove expired events
  gt krc prune --dry-run    # Preview what would be pruned
  gt krc config             # Show TTL configuration
  gt krc config set patrol_* 12h   # Set TTL for patrol events

```
gt krc [flags]
```

### Options

```
  -h, --help   help for krc
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt krc auto-prune-status](../cli/gt_krc_auto-prune-status/)	 - Show auto-prune scheduling state
* [gt krc config](../cli/gt_krc_config/)	 - View or modify TTL configuration
* [gt krc decay](../cli/gt_krc_decay/)	 - Show forensic value decay report
* [gt krc prune](../cli/gt_krc_prune/)	 - Remove expired events
* [gt krc stats](../cli/gt_krc_stats/)	 - Show statistics about ephemeral data

