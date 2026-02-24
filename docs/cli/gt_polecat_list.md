---
title: "GT POLECAT LIST"
---

## gt polecat list

List polecats in a rig

### Synopsis

List polecats in a rig or all rigs.

In the transient model, polecats exist only while working. The list shows
all polecats with their states:
  - working: Actively working on an issue
  - done: Completed work, waiting for cleanup
  - stuck: Needs assistance

Examples:
  gt polecat list greenplace
  gt polecat list --all
  gt polecat list greenplace --json

```
gt polecat list [rig] [flags]
```

### Options

```
      --all    List polecats in all rigs
  -h, --help   help for list
      --json   Output as JSON
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

