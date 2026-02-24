---
title: "GT FORMULA LIST"
---

## gt formula list

List available formulas

### Synopsis

List available formulas from all search paths.

Searches for formula files (.formula.toml, .formula.json) in:
  1. .beads/formulas/ (project)
  2. ~/.beads/formulas/ (user)
  3. $GT_ROOT/.beads/formulas/ (orchestrator)

Examples:
  gt formula list            # List all formulas
  gt formula list --json     # JSON output

```
gt formula list [flags]
```

### Options

```
  -h, --help   help for list
      --json   Output as JSON
```

### SEE ALSO

* [gt formula](../cli/gt_formula/)	 - Manage workflow formulas

