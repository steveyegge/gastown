---
title: "GT HOOKS DIFF"
---

## gt hooks diff

Show what sync would change

### Synopsis

Show what 'gt hooks sync' would change without applying.

Compares the current .claude/settings.json files against what would
be generated from base + overrides. Uses color to highlight additions
and removals.

Exit codes:
  0 - No changes pending
  1 - Changes would be applied

Examples:
  gt hooks diff                    # Show all pending changes
  gt hooks diff gastown/crew       # Show changes for specific target

```
gt hooks diff [target] [flags]
```

### Options

```
  -h, --help   help for diff
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

