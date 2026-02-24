---
title: "DOCS/CLI/GT MOL STEP"
---

## gt mol step

Molecule step operations

### Synopsis

Commands for working with molecule steps.

A molecule is a DAG of steps. Each step is a beads issue with the molecule root
as its parent. Steps can have dependencies on other steps.

When a polecat is working on a molecule, it processes one step at a time:
1. Work on the current step
2. When done: gt mol step done <step-id>
3. System auto-continues to next ready step

IMPORTANT: Always use 'gt mol step done' to complete steps. Do not manually
close steps with 'bd close' - that skips the auto-continuation logic.

```
gt mol step [flags]
```

### Options

```
  -h, --help   help for step
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands
* [gt mol step await-event](../cli/gt_mol_step_await-event/)	 - Wait for a file-based event on a named channel
* [gt mol step await-signal](../cli/gt_mol_step_await-signal/)	 - Wait for activity feed signal with timeout
* [gt mol step done](../cli/gt_mol_step_done/)	 - Complete step and auto-continue to next
* [gt mol step emit-event](../cli/gt_mol_step_emit-event/)	 - Emit a file-based event on a named channel

