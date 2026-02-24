---
title: "DOCS/CLI/GT SESSION RESTART"
---

## gt session restart

Restart a polecat session

### Synopsis

Restart a polecat session (stop + start).

Gracefully stops the current session and starts a fresh one.
Use --force to skip graceful shutdown.

```
gt session restart <rig>/<polecat> [flags]
```

### Options

```
  -f, --force   Force immediate shutdown
  -h, --help    help for restart
```

### SEE ALSO

* [gt session](../cli/gt_session/)	 - Manage polecat sessions

