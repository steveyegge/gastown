---
title: "GT MOL DAG"
---

## gt mol dag

Visualize molecule dependency DAG

### Synopsis

Display the dependency DAG (Directed Acyclic Graph) for a molecule.

Shows the dependency structure with execution tiers and status:
  ✓ done        - Step completed
  ⧖ in_progress - Step being worked on
  ○ ready       - Step ready to execute (all deps met)
  ◌ blocked     - Step waiting on dependencies

Examples:
  gt mol dag gs-wisp-abc     # Show DAG for molecule
  gt mol dag gs-wisp-abc --json  # JSON output
  gt mol dag gs-wisp-abc --tree  # Tree view (default)
  gt mol dag gs-wisp-abc --tiers # Group by execution tier

```
gt mol dag <molecule-id> [flags]
```

### Options

```
  -h, --help    help for dag
      --json    Output as JSON
      --tiers   Group output by execution tier
      --tree    Show tree view (default) (default true)
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

