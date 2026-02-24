---
title: "GT WITNESS RESTART"
---

## gt witness restart

Restart the witness

### Synopsis

Restart the Witness for a rig.

Stops the current session (if running) and starts a fresh one.

Examples:
  gt witness restart greenplace
  gt witness restart greenplace --agent codex
  gt witness restart greenplace --env ANTHROPIC_MODEL=claude-3-haiku

```
gt witness restart <rig> [flags]
```

### Options

```
      --agent string      Agent alias to run the Witness with (overrides town default)
      --env stringArray   Environment variable override (KEY=VALUE, can be repeated)
  -h, --help              help for restart
```

### SEE ALSO

* [gt witness](../cli/gt_witness/)	 - Manage the Witness (per-rig polecat health monitor)

