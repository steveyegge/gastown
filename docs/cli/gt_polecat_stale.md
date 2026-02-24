---
title: "GT POLECAT STALE"
---

## gt polecat stale

Detect stale polecats that may need cleanup

### Synopsis

Detect stale polecats in a rig that are candidates for cleanup.

A polecat is considered stale if:
  - No active tmux session
  - Way behind main (>threshold commits) OR no agent bead
  - Has no uncommitted work that could be lost

The default threshold is 20 commits behind main.

Use --cleanup to automatically nuke stale polecats that are safe to remove.
Use --dry-run with --cleanup to see what would be cleaned.

Examples:
  gt polecat stale greenplace
  gt polecat stale greenplace --threshold 50
  gt polecat stale greenplace --json
  gt polecat stale greenplace --cleanup
  gt polecat stale greenplace --cleanup --dry-run

```
gt polecat stale <rig> [flags]
```

### Options

```
      --cleanup         Automatically nuke stale polecats
      --dry-run         Show what would be cleaned without doing it
  -h, --help            help for stale
      --json            Output as JSON
      --threshold int   Commits behind main to consider stale (default 20)
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

