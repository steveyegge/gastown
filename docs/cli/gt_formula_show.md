---
title: "DOCS/CLI/GT FORMULA SHOW"
---

## gt formula show

Display formula details

### Synopsis

Display detailed information about a formula.

Shows:
  - Formula metadata (name, type, description)
  - Variables with defaults and constraints
  - Steps with dependencies
  - Composition rules (extends, aspects)

Examples:
  gt formula show shiny
  gt formula show rule-of-five --json

```
gt formula show <name> [flags]
```

### Options

```
  -h, --help   help for show
      --json   Output as JSON
```

### SEE ALSO

* [gt formula](../cli/gt_formula/)	 - Manage workflow formulas

