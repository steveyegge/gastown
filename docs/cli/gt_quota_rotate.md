---
title: "GT QUOTA ROTATE"
---

## gt quota rotate

Swap blocked sessions to available accounts

### Synopsis

Rotate rate-limited sessions to available accounts.

Scans all sessions for rate limits, plans account assignments using
least-recently-used ordering, and restarts blocked sessions with fresh accounts.

Use --from to preemptively rotate sessions using a specific account before
it hits its rate limit. This is useful for switching idle sessions while
it's not disruptive.

The rotation process:
  1. Scans all Gas Town sessions for rate-limit indicators
  2. Selects available accounts (LRU order)
  3. Swaps macOS Keychain credentials (same config dir preserved)
  4. Restarts blocked sessions via respawn-pane
  5. Sends /resume to recover conversation context

Examples:
  gt quota rotate                    # Rotate all blocked sessions
  gt quota rotate --from work        # Preemptively rotate sessions on 'work' account
  gt quota rotate --from work --idle # Only rotate idle sessions on 'work' account
  gt quota rotate --dry-run          # Show plan without executing
  gt quota rotate --json             # JSON output

```
gt quota rotate [flags]
```

### Options

```
      --dry-run       Show plan without executing
      --from string   Preemptively rotate sessions using this account
  -h, --help          help for rotate
      --idle          Only rotate sessions at the idle prompt (skip busy agents)
      --json          Output as JSON
```

### SEE ALSO

* [gt quota](../cli/gt_quota/)	 - Manage account quota rotation

