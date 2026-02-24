---
title: "DOCS/CLI/GT TRAIL"
---

## gt trail

Show recent agent activity

### Synopsis

Show recent activity in the workspace.

Without a subcommand, shows recent commits from agents.

Subcommands:
  commits    Recent git commits from agents
  beads      Recent beads (work items)
  hooks      Recent hook activity

Flags:
  --since    Show activity since this time (e.g., "1h", "24h", "7d")
  --limit    Maximum number of items to show (default: 20)
  --json     Output as JSON
  --all      Include all activity (not just agents)

Examples:
  gt trail                     # Recent commits (default)
  gt trail commits             # Same as above
  gt trail commits --since 1h  # Last hour
  gt trail beads               # Recent beads
  gt trail hooks               # Recent hook activity
  gt recent                    # Alias for gt trail
  gt recap --since 24h         # Activity from last 24 hours

```
gt trail [flags]
```

### Options

```
      --all            Include all activity (not just agents)
  -h, --help           help for trail
      --json           Output as JSON
      --limit int      Maximum number of items to show (default 20)
      --since string   Show activity since this time (e.g., 1h, 24h, 7d)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt trail beads](../cli/gt_trail_beads/)	 - Show recent beads
* [gt trail commits](../cli/gt_trail_commits/)	 - Show recent commits from agents
* [gt trail hooks](../cli/gt_trail_hooks/)	 - Show recent hook activity

