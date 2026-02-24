---
title: "GT ORPHANS PROCS LIST"
---

## gt orphans procs list

List orphaned Claude processes

### Synopsis

List Claude processes that have become orphaned (PPID=1).

These are processes that survived session termination and are now
parented to init/launchd. They consume resources and should be killed.

Use --aggressive to detect ALL orphaned Claude processes by cross-referencing
against active tmux sessions. Any Claude process NOT in a gt-* or hq-* session
is considered an orphan.

Excludes:
- tmux server processes
- Claude.app desktop application processes

Examples:
  gt orphans procs list             # Show orphans with PPID=1
  gt orphans procs list --aggressive # Show ALL orphans (tmux verification)

```
gt orphans procs list [flags]
```

### Options

```
  -h, --help   help for list
```

### Options inherited from parent commands

```
      --aggressive   Use tmux session verification to find ALL orphans (not just PPID=1)
      --rig string   Target rig name (required when not in a rig directory)
```

### SEE ALSO

* [gt orphans procs](../cli/gt_orphans_procs/)	 - Manage orphaned Claude processes

