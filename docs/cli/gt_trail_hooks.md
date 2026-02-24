---
title: "GT TRAIL HOOKS"
---

## gt trail hooks

Show recent hook activity

### Synopsis

Show recent hook activity (agents taking or dropping hooks).

Examples:
  gt trail hooks              # Recent hook activity
  gt trail hooks --since 1h   # Last hour of hook activity
  gt trail hooks --json       # JSON output

```
gt trail hooks [flags]
```

### Options

```
  -h, --help   help for hooks
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

