---
title: "GT KRC DECAY"
---

## gt krc decay

Show forensic value decay report

### Synopsis

Display how forensic value is decaying across event types.

Each event type has a decay curve that models how its value diminishes over time:
  rapid   - value drops quickly (heartbeats, pings)
  steady  - linear decay (session events, patrols)
  slow    - value persists longer (errors, escalations)
  flat    - full value until near TTL (audit events, deaths)

Events with low forensic scores are candidates for aggressive pruning.

```
gt krc decay [flags]
```

### Options

```
  -h, --help   help for decay
      --json   Output in JSON format
```

### SEE ALSO

* [gt krc](../cli/gt_krc/)	 - Key Record Chronicle - manage ephemeral data TTLs

