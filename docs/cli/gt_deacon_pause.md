---
title: "DOCS/CLI/GT DEACON PAUSE"
---

## gt deacon pause

Pause the Deacon to prevent patrol actions

### Synopsis

Pause the Deacon to prevent it from performing any patrol actions.

When paused, the Deacon:
- Will not create patrol molecules
- Will not run health checks
- Will not take any autonomous actions
- Will display a PAUSED message on startup

The pause state persists across session restarts. Use 'gt deacon resume'
to allow the Deacon to work again.

Examples:
  gt deacon pause                           # Pause with no reason
  gt deacon pause --reason="testing"        # Pause with a reason

```
gt deacon pause [flags]
```

### Options

```
  -h, --help            help for pause
      --reason string   Reason for pausing the Deacon
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

