---
title: "GT DEACON REDISPATCH"
---

## gt deacon redispatch

Re-dispatch a recovered bead to an available polecat

### Synopsis

Re-dispatch a recovered bead from a dead polecat to an available polecat.

When the Witness detects a dead polecat with abandoned work, it resets the bead
to open status and sends a RECOVERED_BEAD mail to the Deacon. This command
handles the re-dispatch:

1. Checks re-dispatch state (how many times this bead has been re-dispatched)
2. Rate-limits to prevent thrashing (cooldown between re-dispatches)
3. If under the limit: runs 'gt sling <bead> <rig>' to re-dispatch
4. If over the limit: escalates to Mayor instead of re-slinging

Exit codes:
  0 - Bead successfully re-dispatched or escalated
  1 - Error occurred
  2 - Bead in cooldown (try again later)
  3 - Bead skipped (already claimed or non-open status)

Examples:
  gt deacon redispatch gt-abc123                    # Auto-detect rig from prefix
  gt deacon redispatch gt-abc123 --rig gastown      # Explicit target rig
  gt deacon redispatch gt-abc123 --max-attempts 5   # Allow 5 attempts before escalation
  gt deacon redispatch gt-abc123 --cooldown 10m     # 10 minute cooldown between attempts

```
gt deacon redispatch <bead-id> [flags]
```

### Options

```
      --cooldown duration   Minimum time between re-dispatches of same bead (default: 5m)
  -h, --help                help for redispatch
      --max-attempts int    Max re-dispatch attempts before escalating to Mayor (default: 3)
      --rig string          Target rig to re-dispatch to (auto-detected from bead prefix if omitted)
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

