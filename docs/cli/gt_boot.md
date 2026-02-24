---
title: "DOCS/CLI/GT BOOT"
---

## gt boot

Manage Boot (Deacon watchdog)

### Synopsis

Manage Boot - the daemon's watchdog for Deacon triage.

Boot is a special dog that runs fresh on each daemon tick. It observes
the system state and decides whether to start/wake/nudge/interrupt the
Deacon, or do nothing. This centralizes the "when to wake" decision in
an agent that can reason about it.

Boot lifecycle:
  1. Daemon tick spawns Boot (fresh each time)
  2. Boot runs triage: observe, decide, act
  3. Boot cleans inbox (discards stale handoffs)
  4. Boot exits (or handoffs in non-degraded mode)

Location: ~/gt/deacon/dogs/boot/
Session: gt-boot

### Options

```
  -h, --help   help for boot
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt boot spawn](../cli/gt_boot_spawn/)	 - Spawn Boot for triage
* [gt boot status](../cli/gt_boot_status/)	 - Show Boot status
* [gt boot triage](../cli/gt_boot_triage/)	 - Run triage directly (degraded mode)

