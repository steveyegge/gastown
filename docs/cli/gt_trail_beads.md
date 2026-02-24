---
title: "DOCS/CLI/GT TRAIL BEADS"
---

## gt trail beads

Show recent beads

### Synopsis

Show recently created or modified beads (work items).

Examples:
  gt trail beads              # Recent beads
  gt trail beads --since 24h  # Last 24 hours of beads
  gt trail beads --json       # JSON output

```
gt trail beads [flags]
```

### Options

```
  -h, --help   help for beads
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

