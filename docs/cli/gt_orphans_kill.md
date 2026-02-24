---
title: "GT ORPHANS KILL"
---

## gt orphans kill

Remove all orphans (commits and processes)

### Synopsis

Remove orphaned commits and kill orphaned Claude processes.

This command performs a complete orphan cleanup:
1. Finds and removes orphaned commits via 'git gc --prune=now'
2. Finds and kills orphaned Claude processes (PPID=1)

WARNING: This operation is irreversible. Once commits are pruned,
they cannot be recovered.

Note: This only affects orphaned commits and processes. Unmerged polecat
branches (shown by 'gt orphans') must be recovered or cleaned up manually.

The command will:
1. Find orphaned commits (same as 'gt orphans')
2. Find orphaned Claude processes (same as 'gt orphans procs')
3. Show what will be removed/killed
4. Ask for confirmation (unless --force)
5. Run git gc and kill processes

Examples:
  gt orphans kill              # Kill orphans from last 7 days (default)
  gt orphans kill --days=14    # Kill orphans from last 2 weeks
  gt orphans kill --all        # Kill all orphans
  gt orphans kill --dry-run    # Preview without deleting
  gt orphans kill --force      # Skip confirmation prompt

```
gt orphans kill [flags]
```

### Options

```
      --all        Kill all orphans (no date filter)
      --days int   Kill orphans from last N days (default 7)
      --dry-run    Preview without deleting
      --force      Skip confirmation prompt
  -h, --help       help for kill
```

### Options inherited from parent commands

```
      --rig string   Target rig name (required when not in a rig directory)
```

### SEE ALSO

* [gt orphans](../cli/gt_orphans/)	 - Find lost polecat work

