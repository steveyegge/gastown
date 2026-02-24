---
title: "GT START"
---

## gt start

Start Gas Town or a crew workspace

### Synopsis

Start Gas Town by launching the Deacon and Mayor.

The Deacon is the health-check orchestrator that monitors Mayor and Witnesses.
The Mayor is the global coordinator that dispatches work.

By default, other agents (Witnesses, Refineries) are started lazily as needed.
Use --all to start Witnesses and Refineries for all registered rigs immediately.

Crew shortcut:
  If a path like "rig/crew/name" is provided, starts that crew workspace.
  This is equivalent to 'gt start crew rig/name'.

To stop Gas Town, use 'gt shutdown'.

```
gt start [path] [flags]
```

### Options

```
      --agent string       Agent alias to run Mayor/Deacon with (overrides town default)
  -a, --all                Also start Witnesses and Refineries for all rigs
      --cost-tier string   Ephemeral cost tier for this session (standard/economy/budget)
  -h, --help               help for start
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt start crew](../cli/gt_start_crew/)	 - Start a crew workspace (creates if needed)

