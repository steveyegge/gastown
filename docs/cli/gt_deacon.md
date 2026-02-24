---
title: "DOCS/CLI/GT DEACON"
---

## gt deacon

Manage the Deacon (town-level watchdog)

### Synopsis

Manage the Deacon - the town-level watchdog for Gas Town.

The Deacon ("daemon beacon") is the only agent that receives mechanical
heartbeats from the daemon. It monitors system health across all rigs:
  - Watches all Witnesses (are they alive? stuck? responsive?)
  - Manages Dogs for cross-rig infrastructure work
  - Handles lifecycle requests (respawns, restarts)
  - Receives heartbeat pokes and decides what needs attention

The Deacon patrols the town; Witnesses patrol their rigs; Polecats work.

Role shortcuts: "deacon" in mail/nudge addresses resolves to this agent.

```
gt deacon [flags]
```

### Options

```
  -h, --help   help for deacon
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt deacon attach](../cli/gt_deacon_attach/)	 - Attach to the Deacon session
* [gt deacon cleanup-orphans](../cli/gt_deacon_cleanup-orphans/)	 - Clean up orphaned claude subagent processes
* [gt deacon feed-stranded](../cli/gt_deacon_feed-stranded/)	 - Detect and feed stranded convoys automatically
* [gt deacon feed-stranded-state](../cli/gt_deacon_feed-stranded-state/)	 - Show feed-stranded state for tracked convoys
* [gt deacon force-kill](../cli/gt_deacon_force-kill/)	 - Force-kill an unresponsive agent session
* [gt deacon health-check](../cli/gt_deacon_health-check/)	 - Send a health check ping to an agent and track response
* [gt deacon health-state](../cli/gt_deacon_health-state/)	 - Show health check state for all monitored agents
* [gt deacon heartbeat](../cli/gt_deacon_heartbeat/)	 - Update the Deacon heartbeat
* [gt deacon pause](../cli/gt_deacon_pause/)	 - Pause the Deacon to prevent patrol actions
* [gt deacon redispatch](../cli/gt_deacon_redispatch/)	 - Re-dispatch a recovered bead to an available polecat
* [gt deacon redispatch-state](../cli/gt_deacon_redispatch-state/)	 - Show re-dispatch state for recovered beads
* [gt deacon restart](../cli/gt_deacon_restart/)	 - Restart the Deacon session
* [gt deacon resume](../cli/gt_deacon_resume/)	 - Resume the Deacon to allow patrol actions
* [gt deacon stale-hooks](../cli/gt_deacon_stale-hooks/)	 - Find and unhook stale hooked beads
* [gt deacon start](../cli/gt_deacon_start/)	 - Start the Deacon session
* [gt deacon status](../cli/gt_deacon_status/)	 - Check Deacon session status
* [gt deacon stop](../cli/gt_deacon_stop/)	 - Stop the Deacon session
* [gt deacon zombie-scan](../cli/gt_deacon_zombie-scan/)	 - Find and clean zombie Claude processes not in active tmux sessions

