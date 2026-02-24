---
title: "DOCS/CLI/GT HOOK SHOW"
---

## gt hook show

Show what's on an agent's hook (compact)

### Synopsis

Show what's on any agent's hook in compact one-line format.

With no argument, shows your own hook status (auto-detected from context).

Use cases:
- Mayor checking what polecats are working on
- Witness checking polecat status
- Debugging coordination issues
- Quick status overview

Examples:
  gt hook show                         # What's on MY hook? (auto-detect)
  gt hook show gastown/polecats/nux    # What's nux working on?
  gt hook show gastown/witness         # What's the witness hooked to?
  gt hook show mayor                   # What's the mayor working on?

Output format (one line):
  gastown/polecats/nux: gt-abc123 'Fix the widget bug' [in_progress]

```
gt hook show [agent] [flags]
```

### Options

```
  -h, --help   help for show
      --json   Output as JSON
```

### SEE ALSO

* [gt hook](../cli/gt_hook/)	 - Show or attach work on a hook

