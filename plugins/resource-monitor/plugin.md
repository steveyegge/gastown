+++
name = "resource-monitor"
description = "Monitor disk, memory, and working polecat count — escalate on threshold breach"
version = 1

[gate]
type = "cooldown"
duration = "15m"

[tracking]
labels = ["plugin:resource-monitor", "category:health"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "high"
+++

# Resource Monitor

Checks disk usage, memory availability, and working polecat count.
Escalates via `gt escalate` if thresholds are breached.

## Thresholds

- Disk usage > 80% → HIGH escalation
- Memory available < 10Gi → HIGH escalation
- Working polecats > 22 → MEDIUM escalation

## Usage

```bash
./run.sh                    # Normal execution
./run.sh --rig <rig>        # Monitor polecats in a specific rig
```
