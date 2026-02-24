---
title: "GT MOL CURRENT"
---

## gt mol current

Show what agent should be working on

### Synopsis

Query what an agent is supposed to be working on via breadcrumb trail.

Looks up the agent's handoff bead, checks for attached molecules, and
identifies the current/next step in the workflow.

If no identity is specified, uses the current agent based on working directory.

Output includes:
- Identity and handoff bead info
- Attached molecule (if any)
- Progress through steps
- Current step that should be worked on next

Examples:
  gt molecule current                 # Current agent's work
  gt molecule current greenplace/furiosa
  gt molecule current deacon
  gt mol current greenplace/witness

```
gt mol current [identity] [flags]
```

### Options

```
  -h, --help   help for current
      --json   Output as JSON
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

