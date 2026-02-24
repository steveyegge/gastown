---
title: "DOCS/CLI/GT QUOTA CLEAR"
---

## gt quota clear

Mark account(s) as available again

### Synopsis

Clear the rate-limited status for one or more accounts, marking them available.

When no handles are specified, all limited accounts are cleared.

Examples:
  gt quota clear              # Clear all limited accounts
  gt quota clear work         # Clear a specific account
  gt quota clear work personal

```
gt quota clear [handle...] [flags]
```

### Options

```
  -h, --help   help for clear
```

### SEE ALSO

* [gt quota](../cli/gt_quota/)	 - Manage account quota rotation

