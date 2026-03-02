---
title: "GT POLECAT CHECK-RECOVERY"
---

## gt polecat check-recovery

Check if polecat needs recovery vs safe to nuke

### Synopsis

Check recovery status of a polecat based on cleanup_status and merge queue state.

Used by the Witness to determine appropriate cleanup action:
  - SAFE_TO_NUKE: cleanup_status is 'clean' AND work submitted to merge queue
  - NEEDS_MQ_SUBMIT: git is clean but work was never submitted to the merge queue
  - NEEDS_RECOVERY: cleanup_status indicates unpushed/uncommitted work

This prevents accidental data loss when cleaning up dormant polecats.
The Witness should escalate NEEDS_RECOVERY and NEEDS_MQ_SUBMIT cases to the Mayor.

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

