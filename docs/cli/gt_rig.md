---
title: "GT RIG"
---

## gt rig

Manage rigs in the workspace

### Synopsis

Manage rigs (project containers) in the Gas Town workspace.

A rig is a container for managing a project and its agents:
  - refinery/rig/  Canonical main clone (Refinery's working copy)
  - mayor/rig/     Mayor's working clone for this rig
  - crew/<name>/   Human workspace(s)
  - witness/       Witness agent (no clone)
  - polecats/      Worker directories
  - .beads/        Rig-level issue tracking

```
gt rig [flags]
```

### Options

```
  -h, --help   help for rig
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt rig add](../cli/gt_rig_add/)	 - Add a new rig to the workspace
* [gt rig boot](../cli/gt_rig_boot/)	 - Start witness and refinery for a rig
* [gt rig config](../cli/gt_rig_config/)	 - View and manage rig configuration
* [gt rig dock](../cli/gt_rig_dock/)	 - Dock a rig (global, persistent shutdown)
* [gt rig list](../cli/gt_rig_list/)	 - List all rigs in the workspace
* [gt rig park](../cli/gt_rig_park/)	 - Park one or more rigs (stops agents, daemon won't auto-restart)
* [gt rig reboot](../cli/gt_rig_reboot/)	 - Restart witness and refinery for a rig
* [gt rig remove](../cli/gt_rig_remove/)	 - Remove a rig from the registry (does not delete files)
* [gt rig reset](../cli/gt_rig_reset/)	 - Reset rig state (handoff content, mail, stale issues)
* [gt rig restart](../cli/gt_rig_restart/)	 - Restart one or more rigs (stop then start)
* [gt rig settings](../cli/gt_rig_settings/)	 - View and manage rig settings
* [gt rig shutdown](../cli/gt_rig_shutdown/)	 - Gracefully stop all rig agents
* [gt rig start](../cli/gt_rig_start/)	 - Start witness and refinery on patrol for one or more rigs
* [gt rig status](../cli/gt_rig_status/)	 - Show detailed status for a specific rig
* [gt rig stop](../cli/gt_rig_stop/)	 - Stop one or more rigs (shutdown semantics)
* [gt rig undock](../cli/gt_rig_undock/)	 - Undock a rig (remove global docked status)
* [gt rig unpark](../cli/gt_rig_unpark/)	 - Unpark one or more rigs (allow daemon to auto-restart agents)

