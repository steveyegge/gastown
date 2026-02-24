---
title: "GT TRAIL COMMITS"
---

## gt trail commits

Show recent commits from agents

### Synopsis

Show recent git commits made by agents.

By default, filters to commits from agents (using the configured
email domain). Use --all to include all commits.

Examples:
  gt trail commits              # Recent agent commits
  gt trail commits --since 1h   # Last hour of commits
  gt trail commits --all        # All commits (including non-agents)
  gt trail commits --json       # JSON output

```
gt trail commits [flags]
```

### Options

```
  -h, --help   help for commits
```

### Options inherited from parent commands

```
      --all            Include all activity (not just agents)
      --json           Output as JSON
      --limit int      Maximum number of items to show (default 20)
      --since string   Show activity since this time (e.g., 1h, 24h, 7d)
```

### SEE ALSO

* [gt trail](../cli/gt_trail/)	 - Show recent agent activity

