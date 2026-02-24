---
title: "DOCS/CLI/GT QUOTA STATUS"
---

## gt quota status

Show account quota status

### Synopsis

Show the quota status of all registered accounts.

Displays which accounts are available, rate-limited, or in cooldown,
along with timestamps for limit detection and estimated reset times.

Examples:
  gt quota status           # Text output
  gt quota status --json    # JSON output

```
gt quota status [flags]
```

### Options

```
  -h, --help   help for status
      --json   Output as JSON
```

### SEE ALSO

* [gt quota](../cli/gt_quota/)	 - Manage account quota rotation

