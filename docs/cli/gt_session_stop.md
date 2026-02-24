---
title: "GT SESSION STOP"
---

## gt session stop

Stop a polecat session

### Synopsis

Stop a running polecat session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.
Use --force to skip graceful shutdown.

```
gt session stop <rig>/<polecat> [flags]
```

### Options

```
  -f, --force   Force immediate shutdown
  -h, --help    help for stop
```

### SEE ALSO

* [gt session](../cli/gt_session/)	 - Manage polecat sessions

