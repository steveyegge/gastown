---
title: "GT QUOTA ROTATE"
---

## gt quota rotate

Swap blocked sessions to available accounts

### Synopsis

Rotate rate-limited sessions to available accounts.

Scans all sessions for rate limits, plans account assignments using
least-recently-used ordering, and restarts blocked sessions with fresh accounts.

The rotation process:
  1. Scans all Gas Town sessions for rate-limit indicators
  2. Selects available accounts (LRU order)
  3. Updates tmux session environment with new CLAUDE_CONFIG_DIR
  4. Restarts blocked sessions via respawn-pane

Examples:
  gt quota rotate              # Rotate all blocked sessions
  gt quota rotate --dry-run    # Show plan without executing
  gt quota rotate --json       # JSON output

```
gt quota rotate [flags]
```

### Options

```
      --dry-run   Show plan without executing
  -h, --help      help for rotate
      --json      Output as JSON
```

### SEE ALSO

* [gt quota](../cli/gt_quota/)	 - Manage account quota rotation

