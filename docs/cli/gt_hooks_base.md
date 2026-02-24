---
title: "DOCS/CLI/GT HOOKS BASE"
---

## gt hooks base

Edit the shared base hook config

### Synopsis

Edit the shared base hook configuration.

The base config defines hooks that apply to all agents. It is stored
at ~/.gt/hooks-base.json. If the file doesn't exist, it will be
created with sensible defaults (PATH setup, gt prime, etc.).

After editing, run 'gt hooks sync' to propagate changes.

Examples:
  gt hooks base           # Open base config in $EDITOR
  gt hooks base --show    # Print current base config to stdout

```
gt hooks base [flags]
```

### Options

```
  -h, --help   help for base
      --show   Print current base config to stdout
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

