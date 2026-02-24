---
title: "GT HOOKS INIT"
---

## gt hooks init

Bootstrap base config from existing settings.json files

### Synopsis

Bootstrap the hooks base config by analyzing existing settings.json files.

This scans all managed .claude/settings.json files in the workspace,
finds hooks that are common across all targets (becomes the base config),
and identifies per-target differences (becomes overrides).

After init, run 'gt hooks diff' to verify no changes would be made.

Examples:
  gt hooks init             # Bootstrap base and overrides
  gt hooks init --dry-run   # Show what would be written without writing

```
gt hooks init [flags]
```

### Options

```
      --dry-run   Show what would be written without writing
  -h, --help      help for init
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

