---
title: "DOCS/CLI/GT HOOKS LIST"
---

## gt hooks list

Show all managed settings.json locations

### Synopsis

Show all managed .claude/settings.json locations and their sync status.

Displays each target with its override chain and whether it is
currently in sync with the base + overrides configuration.

Examples:
  gt hooks list            # Show all managed locations
  gt hooks list --json     # Output as JSON

```
gt hooks list [flags]
```

### Options

```
  -h, --help   help for list
      --json   Output as JSON
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

