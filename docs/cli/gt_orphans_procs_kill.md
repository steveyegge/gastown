---
title: "DOCS/CLI/GT ORPHANS PROCS KILL"
---

## gt orphans procs kill

Kill orphaned Claude processes

### Synopsis

Kill Claude processes that have become orphaned (PPID=1).

Without flags, prompts for confirmation before killing.
Use -f/--force to kill without confirmation.
Use --aggressive to kill ALL orphaned processes (not just PPID=1).

Examples:
  gt orphans procs kill             # Kill with confirmation
  gt orphans procs kill -f          # Force kill without confirmation
  gt orphans procs kill --aggressive # Kill ALL orphans (tmux verification)

```
gt orphans procs kill [flags]
```

### Options

```
  -f, --force   Kill without confirmation
  -h, --help    help for kill
```

### Options inherited from parent commands

```
      --aggressive   Use tmux session verification to find ALL orphans (not just PPID=1)
      --rig string   Target rig name (required when not in a rig directory)
```

### SEE ALSO

* [gt orphans procs](../cli/gt_orphans_procs/)	 - Manage orphaned Claude processes

