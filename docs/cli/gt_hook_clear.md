---
title: "DOCS/CLI/GT HOOK CLEAR"
---

## gt hook clear

Clear your hook (alias for 'gt unhook')

### Synopsis

Remove work from your hook (alias for 'gt unhook').

With no arguments, clears your own hook. With a bead ID, only clears
if that specific bead is currently hooked. With a target, operates on
another agent's hook.

Examples:
  gt hook clear                       # Clear my hook (whatever's there)
  gt hook clear gt-abc                # Only clear if gt-abc is hooked
  gt hook clear greenplace/joe        # Clear joe's hook

Related commands:
  gt unhook           # Same as 'gt hook clear'
  gt unsling          # Same as 'gt hook clear'

```
gt hook clear [bead-id] [target] [flags]
```

### Options

```
  -n, --dry-run   Show what would be done
  -f, --force     Clear even if work is incomplete
  -h, --help      help for clear
```

### SEE ALSO

* [gt hook](../cli/gt_hook/)	 - Show or attach work on a hook

