---
title: "DOCS/CLI/GT ORPHANS PROCS"
---

## gt orphans procs

Manage orphaned Claude processes

### Synopsis

Find and kill Claude processes that have become orphaned (PPID=1).

These are processes that survived session termination and are now
parented to init/launchd. They consume resources and should be killed.

Use --aggressive to detect ALL orphaned Claude processes by cross-referencing
against active tmux sessions. Any Claude process NOT in a gt-* or hq-* session
is considered an orphan. This catches processes that have been reparented to
something other than init (PPID != 1).

Examples:
  gt orphans procs              # List orphaned Claude processes (PPID=1 only)
  gt orphans procs list         # Same as above
  gt orphans procs --aggressive # List ALL orphaned processes (tmux verification)
  gt orphans procs kill         # Kill orphaned processes

```
gt orphans procs [flags]
```

### Options

```
      --aggressive   Use tmux session verification to find ALL orphans (not just PPID=1)
  -h, --help         help for procs
```

### Options inherited from parent commands

```
      --rig string   Target rig name (required when not in a rig directory)
```

### SEE ALSO

* [gt orphans](../cli/gt_orphans/)	 - Find lost polecat work
* [gt orphans procs kill](../cli/gt_orphans_procs_kill/)	 - Kill orphaned Claude processes
* [gt orphans procs list](../cli/gt_orphans_procs_list/)	 - List orphaned Claude processes

