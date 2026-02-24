---
title: "GT FORMULA RUN"
---

## gt formula run

Execute a formula

### Synopsis

Execute a formula by pouring it and dispatching work.

This command:
  1. Looks up the formula by name (or uses default from rig config)
  2. Pours it to create a molecule (or uses existing proto)
  3. Dispatches the molecule to available workers

For PR-based workflows, use --pr to specify the GitHub PR number.

If no formula name is provided, uses the default formula configured in
the rig's settings/config.json under workflow.default_formula.

Options:
  --pr=N      Run formula on GitHub PR #N
  --rig=NAME  Target specific rig (default: current or gastown)
  --dry-run   Show what would happen without executing

Examples:
  gt formula run shiny                    # Run formula in current rig
  gt formula run                          # Run default formula from rig config
  gt formula run shiny --pr=123           # Run on PR #123
  gt formula run security-audit --rig=beads  # Run in specific rig
  gt formula run release --dry-run        # Preview execution

```
gt formula run [name] [flags]
```

### Options

```
      --dry-run      Preview execution without running
  -h, --help         help for run
      --pr int       GitHub PR number to run formula on
      --rig string   Target rig (default: current or gastown)
```

### SEE ALSO

* [gt formula](../cli/gt_formula/)	 - Manage workflow formulas

