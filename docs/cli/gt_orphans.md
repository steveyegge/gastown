---
title: "DOCS/CLI/GT ORPHANS"
---

## gt orphans

Find lost polecat work

### Synopsis

Find orphaned commits and unmerged polecat branches.

Polecat work can get lost when:
- Session killed before merge
- Refinery fails to process
- Network issues during push

This command scans for:
1. Orphaned commits via 'git fsck --unreachable' (filtered by --days/--all)
2. Unmerged polecat worktree branches (always shown)

Note: --days and --all only apply to orphaned commits, not polecat branches.

Examples:
  gt orphans              # Last 7 days (default), infers rig from cwd
  gt orphans --rig=gastown # Target a specific rig
  gt orphans --days=14    # Last 2 weeks
  gt orphans --all        # Show all orphans (no date filter)

```
gt orphans [flags]
```

### Options

```
      --all          Show all orphans (no date filter)
      --days int     Show orphans from last N days (default 7)
  -h, --help         help for orphans
      --rig string   Target rig name (required when not in a rig directory)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt orphans kill](../cli/gt_orphans_kill/)	 - Remove all orphans (commits and processes)
* [gt orphans procs](../cli/gt_orphans_procs/)	 - Manage orphaned Claude processes

