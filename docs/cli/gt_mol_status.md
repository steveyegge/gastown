---
title: "DOCS/CLI/GT MOL STATUS"
---

## gt mol status

Show what's on an agent's hook

### Synopsis

Show what's slung on an agent's hook.

If no target is specified, shows the current agent's status based on
the working directory (polecat, crew member, witness, etc.).

Output includes:
- What's slung (molecule name, associated issue)
- Current phase and progress
- Whether it's a wisp
- Next action hint

Examples:
  gt mol status                       # Show current agent's hook
  gt mol status greenplace/nux        # Show specific polecat's hook
  gt mol status greenplace/witness    # Show witness's hook

```
gt mol status [target] [flags]
```

### Options

```
  -h, --help   help for status
      --json   Output as JSON
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

