---
title: "DOCS/CLI/GT HOOKS REGISTRY"
---

## gt hooks registry

List available hooks from the registry

### Synopsis

List all hooks defined in the hook registry.

The registry is at ~/gt/hooks/registry.toml and defines hooks that can be
installed for different roles (crew, polecat, witness, etc.).

Examples:
  gt hooks registry           # Show enabled hooks
  gt hooks registry --all     # Show all hooks including disabled

```
gt hooks registry [flags]
```

### Options

```
  -a, --all       Show all hooks including disabled
  -h, --help      help for registry
  -v, --verbose   Show hook commands and matchers
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

