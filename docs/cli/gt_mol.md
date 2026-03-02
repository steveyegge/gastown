---
title: "GT MOL"
---

## gt mol

Agent molecule workflow commands

### Synopsis

Agent-specific molecule workflow operations.

These commands operate on YOUR hook and YOUR attached molecules.
Use 'gt hook' to see what's on your hook (alias for 'gt mol status').

VIEWING YOUR WORK:
  gt hook              Show what's on your hook
  gt mol current       Show what you should be working on
  gt mol progress      Show execution progress

WORKING ON STEPS:
  gt mol step done     Complete current step (auto-continues)

LIFECYCLE:
  gt mol attach        Attach molecule to your hook
  gt mol detach        Detach molecule from your hook
  gt mol burn          Discard attached molecule (no record)
  gt mol squash        Compress to digest (permanent record)

TO DISPATCH WORK (with molecules):
  gt sling mol-xxx target   # Pour formula + sling to agent
  gt formulas               # List available formulas

```
gt mol [flags]
```

### Options

```
  -h, --help   help for mol
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt mol attach](../cli/gt_mol_attach/)	 - Attach a molecule to a pinned bead
* [gt mol attach-from-mail](../cli/gt_mol_attach-from-mail/)	 - Attach a molecule from a mail message
* [gt mol attachment](../cli/gt_mol_attachment/)	 - Show attachment status of a pinned bead
* [gt mol await-signal](../cli/gt_mol_await-signal/)	 - Wait for activity feed signal with timeout (alias: gt mol step await-signal)
* [gt mol burn](../cli/gt_mol_burn/)	 - Burn current molecule without creating a digest
* [gt mol current](../cli/gt_mol_current/)	 - Show what agent should be working on
* [gt mol dag](../cli/gt_mol_dag/)	 - Visualize molecule dependency DAG
* [gt mol detach](../cli/gt_mol_detach/)	 - Detach molecule from a pinned bead
* [gt mol progress](../cli/gt_mol_progress/)	 - Show progress through a molecule's steps
* [gt mol squash](../cli/gt_mol_squash/)	 - Compress molecule into a digest
* [gt mol status](../cli/gt_mol_status/)	 - Show what's on an agent's hook
* [gt mol step](../cli/gt_mol_step/)	 - Molecule step operations

