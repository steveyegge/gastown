---
title: "DOCS/CLI/GT HOOKS INSTALL"
---

## gt hooks install

Install a hook from the registry

### Synopsis

Install a hook from the registry to worktrees.

By default, installs to the current worktree. Use --role to install
to all worktrees of a specific role in the current rig.

Examples:
  gt hooks install pr-workflow-guard              # Install to current worktree
  gt hooks install pr-workflow-guard --role crew  # Install to all crew in current rig
  gt hooks install session-prime --role crew --all-rigs  # Install to all crew everywhere
  gt hooks install pr-workflow-guard --dry-run    # Preview what would be installed

```
gt hooks install <hook-name> [flags]
```

### Options

```
      --all-rigs      Install across all rigs (requires --role)
      --dry-run       Preview changes without writing files
      --force         Install even if hook is disabled in registry
  -h, --help          help for install
      --role string   Install to all worktrees of this role (crew, polecat, witness, refinery)
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

