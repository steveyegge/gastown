---
title: "DOCS/CLI/GT WL CLAIM"
---

## gt wl claim

Claim a wanted item

### Synopsis

Claim a wanted item on the shared wanted board.

Updates the wanted row: claimed_by=<your rig handle>, status='claimed'.
The item must exist and have status='open'.

In wild-west mode (Phase 1), this writes directly to the local wl-commons
database. In PR mode, this will create a DoltHub PR instead.

Examples:
  gt wl claim w-abc123

```
gt wl claim <wanted-id> [flags]
```

### Options

```
  -h, --help   help for claim
```

### SEE ALSO

* [gt wl](../cli/gt_wl/)	 - Wasteland federation commands

