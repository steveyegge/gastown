---
title: "GT READY"
---

## gt ready

Show work ready across town

### Synopsis

Display all ready work items across the town and all rigs.

Aggregates ready issues from:
- Town beads (hq-* items: convoys, cross-rig coordination)
- Each rig's beads (project-level issues, MRs)

Ready items have no blockers and can be worked immediately.
Results are sorted by priority (highest first) then by source.

Examples:
  gt ready              # Show all ready work
  gt ready --json       # Output as JSON
  gt ready --rig=gastown  # Show only one rig

```
gt ready [flags]
```

### Options

```
  -h, --help         help for ready
      --json         Output as JSON
      --rig string   Filter to a specific rig
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

