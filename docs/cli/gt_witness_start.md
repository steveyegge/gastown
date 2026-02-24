---
title: "DOCS/CLI/GT WITNESS START"
---

## gt witness start

Start the witness

### Synopsis

Start the Witness for a rig.

Launches the monitoring agent which watches for stuck polecats and orphaned
sandboxes, taking action to keep work flowing.

Self-Cleaning Model: Polecats nuke themselves after work. The Witness handles
crash recovery (restart with hooked work) and orphan cleanup (nuke abandoned
sandboxes). There is no "idle" state - polecats either have work or don't exist.

Examples:
  gt witness start greenplace
  gt witness start greenplace --agent codex
  gt witness start greenplace --env ANTHROPIC_MODEL=claude-3-haiku
  gt witness start greenplace --foreground

```
gt witness start <rig> [flags]
```

### Options

```
      --agent string      Agent alias to run the Witness with (overrides town default)
      --env stringArray   Environment variable override (KEY=VALUE, can be repeated)
      --foreground        Run in foreground (default: background)
  -h, --help              help for start
```

### SEE ALSO

* [gt witness](../cli/gt_witness/)	 - Manage the Witness (per-rig polecat health monitor)

