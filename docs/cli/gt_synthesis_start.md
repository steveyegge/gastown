---
title: "DOCS/CLI/GT SYNTHESIS START"
---

## gt synthesis start

Start synthesis for a convoy

### Synopsis

Start the synthesis step for a convoy.

This command:
  1. Verifies all legs are complete
  2. Collects outputs from all legs
  3. Creates a synthesis bead with combined context
  4. Slings the synthesis to a polecat

Options:
  --rig=NAME      Target rig for synthesis polecat (default: current)
  --review-id=ID  Override review ID for output paths
  --force         Start synthesis even if some legs incomplete
  --dry-run       Show what would happen without executing

```
gt synthesis start <convoy-id> [flags]
```

### Options

```
      --dry-run            Preview execution
      --force              Start even if legs incomplete
  -h, --help               help for start
      --review-id string   Override review ID
      --rig string         Target rig for synthesis polecat
```

### SEE ALSO

* [gt synthesis](../cli/gt_synthesis/)	 - Manage convoy synthesis steps

