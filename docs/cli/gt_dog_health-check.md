---
title: "DOCS/CLI/GT DOG HEALTH-CHECK"
---

## gt dog health-check

Check dog health (zombies, hung, orphans)

### Synopsis

Check dog health and detect problems.

Detects:
  - Zombies: state=working but tmux session or agent process is dead
  - Hung: agent alive but no tmux activity for too long
  - Orphans: dog idle but tmux session still exists

With --auto-clear, zombies are automatically returned to idle state.
Hung dogs are reported only (Deacon decides per ZFC principle).

Exit codes:
  0 = all healthy
  1 = error
  2 = needs attention

Examples:
  gt dog health-check
  gt dog health-check alpha
  gt dog health-check --json
  gt dog health-check --auto-clear
  gt dog health-check --max-inactivity 1h

```
gt dog health-check [name] [flags]
```

### Options

```
      --auto-clear                Auto-clear zombie dogs
  -h, --help                      help for health-check
      --json                      Output as JSON
      --max-inactivity duration   Max inactivity before considering hung (default 30m0s)
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

