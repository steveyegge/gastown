---
title: "DOCS/CLI/GT RIG PARK"
---

## gt rig park

Park one or more rigs (stops agents, daemon won't auto-restart)

### Synopsis

Park rigs to temporarily disable them.

Parking a rig:
  - Stops the witness if running
  - Stops the refinery if running
  - Sets status=parked in the wisp layer (local/ephemeral)
  - The daemon respects this status and won't auto-restart agents

This is a Level 1 (local/ephemeral) operation:
  - Only affects this town
  - Disappears on wisp cleanup
  - Use 'gt rig unpark' to resume normal operation

Examples:
  gt rig park gastown
  gt rig park beads gastown mayor

```
gt rig park <rig>... [flags]
```

### Options

```
  -h, --help   help for park
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

