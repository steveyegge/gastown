---
title: "GT AGENTS CHECK"
---

## gt agents check

Check for identity collisions and stale locks

### Synopsis

Check for identity collisions and stale locks.

This command helps detect situations where multiple Claude processes
think they own the same worker identity.

Output shows:
  - Active tmux sessions with gt- prefix
  - Identity locks in worker directories
  - Collisions (multiple agents claiming same identity)
  - Stale locks (dead PIDs)

```
gt agents check [flags]
```

### Options

```
  -h, --help   help for check
      --json   Output as JSON
```

### Options inherited from parent commands

```
  -a, --all   Include polecats in the menu
```

### SEE ALSO

* [gt agents](../cli/gt_agents/)	 - List Gas Town agent sessions

