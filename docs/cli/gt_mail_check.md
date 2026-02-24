---
title: "DOCS/CLI/GT MAIL CHECK"
---

## gt mail check

Check for new mail (for hooks)

### Synopsis

Check for new mail - useful for Claude Code hooks.

Exit codes (normal mode):
  0 - New mail available
  1 - No new mail

Exit codes (--inject mode):
  0 - Always (hooks should never block)
  Output: system-reminder if mail exists, silent if no mail

Use --identity for polecats to explicitly specify their identity.

Examples:
  gt mail check                           # Simple check (auto-detect identity)
  gt mail check --inject                  # For hooks
  gt mail check --identity greenplace/Toast  # Explicit polecat identity

```
gt mail check [flags]
```

### Options

```
      --address string    Alias for --identity
  -h, --help              help for check
      --identity string   Explicit identity for inbox (e.g., greenplace/Toast)
      --inject            Output format for Claude Code hooks
      --json              Output as JSON
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

