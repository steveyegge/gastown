---
title: "GT POLECAT CHECK-RECOVERY"
---

## gt polecat check-recovery

Check if polecat needs recovery vs safe to nuke

### Synopsis

Check recovery status of a polecat based on cleanup_status in agent bead.

Used by the Witness to determine appropriate cleanup action:
  - SAFE_TO_NUKE: cleanup_status is 'clean' - no work at risk
  - NEEDS_RECOVERY: cleanup_status indicates unpushed/uncommitted work

This prevents accidental data loss when cleaning up dormant polecats.
The Witness should escalate NEEDS_RECOVERY cases to the Mayor.

Examples:
  gt polecat check-recovery greenplace/Toast
  gt polecat check-recovery greenplace/Toast --json

```
gt polecat check-recovery <rig>/<polecat> [flags]
```

### Options

```
  -h, --help   help for check-recovery
      --json   Output as JSON
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

