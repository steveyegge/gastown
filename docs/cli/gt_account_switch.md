---
title: "GT ACCOUNT SWITCH"
---

## gt account switch

Switch to a different account

### Synopsis

Switch the active Claude Code account.

This command:
1. Backs up ~/.claude to the current account's config_dir (if needed)
2. Creates a symlink from ~/.claude to the target account's config_dir
3. Updates the default account in accounts.json

After switching, you must restart Claude Code for the change to take effect.

Examples:
  gt account switch work       # Switch to work account
  gt account switch personal   # Switch to personal account

```
gt account switch <handle> [flags]
```

### Options

```
  -h, --help   help for switch
```

### SEE ALSO

* [gt account](../cli/gt_account/)	 - Manage Claude Code accounts

