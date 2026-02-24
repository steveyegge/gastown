---
title: "GT CONFIG COST-TIER"
---

## gt config cost-tier

Get or set cost optimization tier

### Synopsis

Get or set the cost optimization tier for model selection.

With no arguments, shows the current cost tier and role assignments.
With an argument, applies the specified tier preset.

Tiers control which AI model each role uses:
  standard  All roles use Opus (highest quality, default)
  economy   Patrol roles use Sonnet/Haiku, workers use Opus
  budget    Patrol roles use Haiku, workers use Sonnet

Examples:
  gt config cost-tier              # Show current tier
  gt config cost-tier economy      # Switch to economy tier
  gt config cost-tier standard     # Reset to all-Opus

```
gt config cost-tier [tier] [flags]
```

### Options

```
  -h, --help   help for cost-tier
```

### SEE ALSO

* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration

