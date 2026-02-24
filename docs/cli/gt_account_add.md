---
title: "GT ACCOUNT ADD"
---

## gt account add

Add a new account

### Synopsis

Add a new Claude Code account.

Creates a config directory at ~/.claude-accounts/<handle> and registers
the account. You'll need to run 'claude' with CLAUDE_CONFIG_DIR set to
that directory to complete the login.

Examples:
  gt account add work
  gt account add work --email steve@company.com
  gt account add work --email steve@company.com --desc "Work account"

```
gt account add <handle> [flags]
```

### Options

```
      --desc string    Account description
      --email string   Account email address
  -h, --help           help for add
```

### SEE ALSO

* [gt account](../cli/gt_account/)	 - Manage Claude Code accounts

