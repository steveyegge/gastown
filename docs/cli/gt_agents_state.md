---
title: "DOCS/CLI/GT AGENTS STATE"
---

## gt agents state

Get or set operational state on agent beads

### Synopsis

Get or set label-based operational state on agent beads.

Agent beads store operational state (like idle cycle counts) as labels.
This command provides a convenient interface for reading and modifying
these labels without affecting other bead properties.

LABEL FORMAT:
Labels are stored as key:value pairs (e.g., idle:3, backoff:2m).

OPERATIONS:
  Get all labels (default):
    gt agent state <agent-bead>

  Set a label:
    gt agent state <agent-bead> --set idle=0
    gt agent state <agent-bead> --set idle=0 --set backoff=30s

  Increment a numeric label:
    gt agent state <agent-bead> --incr idle
    (Creates label with value 1 if not present)

  Delete a label:
    gt agent state <agent-bead> --del idle

COMMON LABELS:
  idle:<n>           - Consecutive idle patrol cycles
  backoff:<duration> - Current backoff interval
  last_activity:<ts> - Last activity timestamp

EXAMPLES:
  # Check current idle count
  gt agent state gt-gastown-witness

  # Reset idle counter after finding work
  gt agent state gt-gastown-witness --set idle=0

  # Increment idle counter on timeout
  gt agent state gt-gastown-witness --incr idle

  # Get state as JSON
  gt agent state gt-gastown-witness --json

```
gt agents state <agent-bead> [flags]
```

### Options

```
      --del stringArray   Delete label (repeatable)
  -h, --help              help for state
      --incr string       Increment numeric label (creates with value 1 if missing)
      --json              Output as JSON
      --set stringArray   Set label value (format: key=value, repeatable)
```

### Options inherited from parent commands

```
  -a, --all   Include polecats in the menu
```

### SEE ALSO

* [gt agents](../cli/gt_agents/)	 - List Gas Town agent sessions

