---
title: "DOCS/CLI/GT HOOK DETACH"
---

## gt hook detach

Detach work from a hook

### Synopsis

Remove a specific bead from a hook (same as 'gt hook clear <bead-id>').

Examples:
  gt hook detach gt-abc               # Detach gt-abc from my hook
  gt hook detach gt-abc gastown/nux   # Detach gt-abc from nux's hook

```
gt hook detach <bead-id> [target] [flags]
```

### Options

```
  -f, --force   Detach even if work is incomplete
  -h, --help    help for detach
```

### SEE ALSO

* [gt hook](../cli/gt_hook/)	 - Show or attach work on a hook

