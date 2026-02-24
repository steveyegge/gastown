---
title: "GT FORMULA"
---

## gt formula

Manage workflow formulas

### Synopsis

Manage workflow formulas - reusable molecule templates.

Formulas are TOML/JSON files that define workflows with steps, variables,
and composition rules. They can be "poured" to create molecules or "wisped"
for ephemeral patrol cycles.

Commands:
  list    List available formulas from all search paths
  show    Display formula details (steps, variables, composition)
  run     Execute a formula (pour and dispatch)
  create  Create a new formula template

Search paths (in order):
  1. .beads/formulas/ (project)
  2. ~/.beads/formulas/ (user)
  3. $GT_ROOT/.beads/formulas/ (orchestrator)

Examples:
  gt formula list                    # List all formulas
  gt formula show shiny              # Show formula details
  gt formula run shiny --pr=123      # Run formula on PR #123
  gt formula create my-workflow      # Create new formula template

```
gt formula [flags]
```

### Options

```
  -h, --help   help for formula
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt formula create](../cli/gt_formula_create/)	 - Create a new formula template
* [gt formula list](../cli/gt_formula_list/)	 - List available formulas
* [gt formula run](../cli/gt_formula_run/)	 - Execute a formula
* [gt formula show](../cli/gt_formula_show/)	 - Display formula details

