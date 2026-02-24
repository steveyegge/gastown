---
title: "DOCS/CLI/GT RIG UNPARK"
---

## gt rig unpark

Unpark one or more rigs (allow daemon to auto-restart agents)

### Synopsis

Unpark rigs to resume normal operation.

Unparking a rig:
  - Removes the parked status from the wisp layer
  - Allows the daemon to auto-restart agents
  - Does NOT automatically start agents (use 'gt rig start' for that)

Examples:
  gt rig unpark gastown
  gt rig unpark beads gastown mayor

```
gt rig unpark <rig>... [flags]
```

### Options

```
  -h, --help   help for unpark
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

