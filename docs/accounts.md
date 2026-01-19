# Claude Account Management

> Managing multiple Claude Code accounts for Gas Town workers

## Overview

Gas Town supports multiple Claude Code accounts, enabling:
- Work/personal account separation
- Different subscription tiers per workload
- Account rotation for rate limiting

Accounts are stored in `~/.claude-accounts/<handle>/` with credentials isolated
per account.

## Quick Reference

```bash
# List accounts
gt account list

# Add new account
gt account add work --email steve@company.com

# Set default
gt account default work

# Check current
gt account status

# Spawn polecat with specific account
gt sling issue-123 myrig --account work
```

## Account Setup

### Adding an Account

```bash
gt account add <handle> [--email <email>] [--desc "description"]
```

This creates:
- Config directory: `~/.claude-accounts/<handle>/`
- Entry in `accounts.json`

**The account is not authenticated yet.** You must complete OAuth:

```bash
CLAUDE_CONFIG_DIR=~/.claude-accounts/<handle> claude
```

This opens the OAuth flow. Complete it in your browser to authenticate.

### Setting the Default Account

```bash
gt account default <handle>
```

New polecats spawned without `--account` will use this account.

### Listing Accounts

```bash
gt account list
```

Shows all registered accounts with the default marked.

## Polecat Account Usage

### Spawning with Specific Account

```bash
gt sling <bead> <rig> --account <handle>
```

The polecat session will have `CLAUDE_CONFIG_DIR` set to the account's config
directory, using that account's credentials.

### Restarting Polecat with Different Account

If a polecat is stuck (OAuth errors, rate limited, etc.) and needs to restart
with a different account:

```bash
# 1. Kill the existing session
tmux kill-session -t gt-<rig>-<polecat>

# 2. Nuke the polecat (use --force if needed, --dry-run first)
gt polecat nuke <rig>/<polecat> --dry-run
gt polecat nuke <rig>/<polecat> --force

# 3. Re-sling with the correct account
gt sling <bead> <rig> --account <handle>
```

**Warning:** `--force` bypasses safety checks. Use `--dry-run` first to see
what would be destroyed.

### OAuth Errors

If a polecat shows OAuth errors like:
```
OAuth error: Invalid code. Please make sure the full code was copied
Press Enter to retry
```

The account's credentials have expired or become invalid. Solutions:

1. **Re-authenticate the account:**
   ```bash
   CLAUDE_CONFIG_DIR=~/.claude-accounts/<handle> claude
   ```
   Complete OAuth flow, then restart the polecat.

2. **Switch to a different account:**
   ```bash
   gt polecat nuke <rig>/<polecat> --force
   gt sling <bead> <rig> --account <other-account>
   ```

## Account Files

Each account directory contains:
```
~/.claude-accounts/<handle>/
├── .claude.json         # Settings
├── .credentials.json    # OAuth tokens (auto-refreshed)
├── projects/            # Project memory
└── ...
```

The `accounts.json` in your Gas Town mayor directory tracks registered accounts:
```json
{
  "version": 1,
  "accounts": {
    "work": {
      "email": "steve@company.com",
      "config_dir": "/home/ubuntu/.claude-accounts/work"
    }
  },
  "default": "work"
}
```

## Environment Variables

When a polecat spawns with an account:
- `CLAUDE_CONFIG_DIR` is set to the account's config directory
- All Claude Code operations use that account's credentials
- OAuth tokens refresh automatically

You can override for testing:
```bash
GT_ACCOUNT=work gt sling ...
```

## Troubleshooting

### Polecat stuck at OAuth prompt
See "OAuth Errors" above. Either re-authenticate or switch accounts.

### Rate limiting
Spawn polecats with different accounts to spread load:
```bash
gt sling issue-1 myrig --account work1
gt sling issue-2 myrig --account work2
```

### Account not found
Ensure the account is added and authenticated:
```bash
gt account list                    # Should show the account
ls ~/.claude-accounts/<handle>/    # Should have .credentials.json
```

### Credentials expired
Re-run OAuth:
```bash
CLAUDE_CONFIG_DIR=~/.claude-accounts/<handle> claude
```
