---
title: "GT SYNTHESIS"
---

## gt synthesis

Manage convoy synthesis steps

### Synopsis

Manage synthesis steps for convoy formulas.

Synthesis is the final step in a convoy workflow that combines outputs
from all parallel legs into a unified deliverable.

Commands:
  start     Start synthesis for a convoy (checks all legs complete)
  status    Show synthesis readiness and leg outputs
  close     Close convoy after synthesis complete

Examples:
  gt synthesis status hq-cv-abc     # Check if ready for synthesis
  gt synthesis start hq-cv-abc      # Start synthesis step
  gt synthesis close hq-cv-abc      # Close convoy after synthesis

```
gt synthesis [flags]
```

### Options

```
  -h, --help   help for synthesis
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt synthesis close](../cli/gt_synthesis_close/)	 - Close convoy after synthesis
* [gt synthesis start](../cli/gt_synthesis_start/)	 - Start synthesis for a convoy
* [gt synthesis status](../cli/gt_synthesis_status/)	 - Show synthesis readiness

