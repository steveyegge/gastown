---
title: "DOCS/CLI/GT ACCOUNT STATUS"
---

## gt account status

Show current account info

### Synopsis

Show which Claude Code account would be used for new sessions.

Displays the currently resolved account based on:
1. GT_ACCOUNT environment variable (highest priority)
2. Default account from config

Examples:
  gt account status           # Show current account
  GT_ACCOUNT=work gt account status  # Show with env override

```
gt account status [flags]
```

### Options

```
  -h, --help   help for status
```

### SEE ALSO

* [gt account](../cli/gt_account/)	 - Manage Claude Code accounts

