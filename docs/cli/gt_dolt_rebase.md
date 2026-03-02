---
title: "GT DOLT REBASE"
---

## gt dolt rebase

Surgical compaction: squash old commits, keep recent ones

### Synopsis

Surgically compact a Dolt database using interactive rebase.

Unlike 'gt dolt flatten' (which destroys ALL history), surgical rebase
keeps recent commits individual while squashing old history into one.

Algorithm (based on Dolt's DOLT_REBASE):
  1. Creates anchor branch at root commit
  2. Creates work branch from main
  3. Starts interactive rebase — populates dolt_rebase table
  4. Marks old commits as 'squash', keeps recent N as 'pick'
  5. Executes the rebase plan
  6. Swaps branches: work becomes the new main
  7. Cleans up temporary branches
  8. Runs GC to reclaim space

WARNING: DOLT_REBASE is NOT safe with concurrent writes. If agents are
actively committing to this database, the rebase may fail with a graph-change
error. The Compactor Dog (daemon) has automatic retry logic for this case.
For manual use, re-run the command if it fails due to concurrent writes.
Flatten mode (gt dolt flatten) is safe with concurrent writes.

Use --keep-recent to control how many recent commits to preserve.
Use --dry-run to see the plan without executing it.

Requires --yes-i-am-sure flag as safety interlock.

```
gt dolt rebase <database> [flags]
```

### Options

```
      --dry-run           Show the rebase plan without executing it
  -h, --help              help for rebase
      --keep-recent int   Number of recent commits to keep as individual picks (default 50)
      --yes-i-am-sure     Required safety flag to confirm compaction
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

