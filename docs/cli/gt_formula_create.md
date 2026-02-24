---
title: "DOCS/CLI/GT FORMULA CREATE"
---

## gt formula create

Create a new formula template

### Synopsis

Create a new formula template file.

Creates a starter formula file in .beads/formulas/ with the given name.
The template includes common sections that you can customize.

Formula types:
  task      Single-step task formula (default)
  workflow  Multi-step workflow with dependencies
  patrol    Repeating patrol cycle (for wisps)

Examples:
  gt formula create my-task                  # Create task formula
  gt formula create my-workflow --type=workflow
  gt formula create nightly-check --type=patrol

```
gt formula create <name> [flags]
```

### Options

```
  -h, --help          help for create
      --type string   Formula type: task, workflow, or patrol (default "task")
```

### SEE ALSO

* [gt formula](../cli/gt_formula/)	 - Manage workflow formulas

