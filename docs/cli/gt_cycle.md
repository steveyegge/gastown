---
title: "DOCS/CLI/GT CYCLE"
---

## gt cycle

Cycle between sessions in the same group

### Synopsis

Cycle between related tmux sessions based on the current session type.

Session groups:
- Town sessions: Mayor ↔ Deacon
- Crew sessions: All crew members in the same rig (e.g., greenplace/crew/max ↔ greenplace/crew/joe)
- Rig infra sessions: Witness ↔ Refinery (per rig)
- Polecat sessions: All polecats in the same rig (e.g., greenplace/Toast ↔ greenplace/Nux)

The appropriate cycling is detected automatically from the session name.

Examples:
  gt cycle next    # Switch to next session in group
  gt cycle prev    # Switch to previous session in group

### Options

```
  -h, --help   help for cycle
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt cycle next](../cli/gt_cycle_next/)	 - Switch to next session in group
* [gt cycle prev](../cli/gt_cycle_prev/)	 - Switch to previous session in group

