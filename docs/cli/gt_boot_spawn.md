---
title: "DOCS/CLI/GT BOOT SPAWN"
---

## gt boot spawn

Spawn Boot for triage

### Synopsis

Spawn Boot to run the triage cycle.

This is normally called by the daemon. It spawns Boot in a fresh
tmux session (or subprocess in degraded mode) to observe and decide
what action to take on the Deacon.

Boot runs to completion and exits - it doesn't maintain state
between invocations.

```
gt boot spawn [flags]
```

### Options

```
      --agent string   Agent alias to run Boot with (overrides town default)
  -h, --help           help for spawn
```

### SEE ALSO

* [gt boot](../cli/gt_boot/)	 - Manage Boot (Deacon watchdog)

