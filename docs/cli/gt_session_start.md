---
title: "DOCS/CLI/GT SESSION START"
---

## gt session start

Start a polecat session

### Synopsis

Start a new tmux session for a polecat.

Creates a tmux session, navigates to the polecat's working directory,
and launches claude. Optionally inject an initial issue to work on.

Examples:
  gt session start wyvern/Toast
  gt session start wyvern/Toast --issue gt-123

```
gt session start <rig>/<polecat> [flags]
```

### Options

```
  -h, --help           help for start
      --issue string   Issue ID to work on
```

### SEE ALSO

* [gt session](../cli/gt_session/)	 - Manage polecat sessions

