---
title: "DOCS/CLI/GT HOOKS SYNC"
---

## gt hooks sync

Regenerate all .claude/settings.json files

### Synopsis

Regenerate all .claude/settings.json files from the base config and overrides.

For each target (mayor, deacon, rig/crew, rig/witness, etc.):
1. Load base config
2. Apply role override (if exists)
3. Apply rig+role override (if exists)
4. Merge hooks section into existing settings.json (preserving all fields)
5. Write updated settings.json

Examples:
  gt hooks sync             # Regenerate all settings.json files
  gt hooks sync --dry-run   # Show what would change without writing

```
gt hooks sync [flags]
```

### Options

```
      --dry-run   Show what would change without writing
  -h, --help      help for sync
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

