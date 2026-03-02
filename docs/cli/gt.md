---
title: "GT"
---

## gt

Gas Town - Multi-agent workspace manager

### Synopsis

Gas Town (gt) manages multi-agent workspaces called rigs.

It coordinates agent spawning, work distribution, and communication
across distributed teams of AI agents working on shared codebases.

### Options

```
  -h, --help   help for gt
```

### SEE ALSO

* [gt account](../cli/gt_account/)	 - Manage Claude Code accounts
* [gt activity](../cli/gt_activity/)	 - Emit and view activity events
* [gt agents](../cli/gt_agents/)	 - List Gas Town agent sessions
* [gt audit](../cli/gt_audit/)	 - Query work history by actor
* [gt bead](../cli/gt_bead/)	 - Bead management utilities
* [gt boot](../cli/gt_boot/)	 - Manage Boot (Deacon watchdog)
* [gt broadcast](../cli/gt_broadcast/)	 - Send a nudge message to all workers
* [gt callbacks](../cli/gt_callbacks/)	 - Handle agent callbacks
* [gt cat](../cli/gt_cat/)	 - Display bead content
* [gt checkpoint](../cli/gt_checkpoint/)	 - Manage session checkpoints for crash recovery
* [gt cleanup](../cli/gt_cleanup/)	 - Clean up orphaned Claude processes
* [gt close](../cli/gt_close/)	 - Close one or more beads
* [gt commit](../cli/gt_commit/)	 - Git commit with automatic agent identity
* [gt compact](../cli/gt_compact/)	 - Compact expired wisps (TTL-based cleanup)
* [gt completion](../cli/gt_completion/)	 - Generate the autocompletion script for the specified shell
* [gt config](../cli/gt_config/)	 - Manage Gas Town configuration
* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs
* [gt costs](../cli/gt_costs/)	 - Show costs for running Claude sessions
* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)
* [gt cycle](../cli/gt_cycle/)	 - Cycle between sessions in the same group
* [gt daemon](../cli/gt_daemon/)	 - Manage the Gas Town daemon
* [gt dashboard](../cli/gt_dashboard/)	 - Start the convoy tracking web dashboard
* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)
* [gt disable](../cli/gt_disable/)	 - Disable Gas Town system-wide
* [gt dnd](../cli/gt_dnd/)	 - Toggle Do Not Disturb mode for notifications
* [gt docgen](../cli/gt_docgen/)	 - Generate Markdown documentation for gt CLI commands
* [gt doctor](../cli/gt_doctor/)	 - Run health checks on the workspace
* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)
* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server
* [gt done](../cli/gt_done/)	 - Signal work ready for merge queue
* [gt down](../cli/gt_down/)	 - Stop all Gas Town services
* [gt enable](../cli/gt_enable/)	 - Enable Gas Town system-wide
* [gt escalate](../cli/gt_escalate/)	 - Escalation system for critical issues
* [gt feed](../cli/gt_feed/)	 - Show real-time activity feed of gt events
* [gt formula](../cli/gt_formula/)	 - Manage workflow formulas
* [gt git-init](../cli/gt_git-init/)	 - Initialize git repository for a Gas Town HQ
* [gt handoff](../cli/gt_handoff/)	 - Hand off to a fresh session, work continues from hook
* [gt health](../cli/gt_health/)	 - Show comprehensive system health
* [gt hook](../cli/gt_hook/)	 - Show or attach work on a hook
* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town
* [gt info](../cli/gt_info/)	 - Show Gas Town information and what's new
* [gt init](../cli/gt_init/)	 - Initialize current directory as a Gas Town rig
* [gt install](../cli/gt_install/)	 - Create a new Gas Town HQ (workspace)
* [gt issue](../cli/gt_issue/)	 - Manage current issue for status line display
* [gt krc](../cli/gt_krc/)	 - Key Record Chronicle - manage ephemeral data TTLs
* [gt log](../cli/gt_log/)	 - View town activity log
* [gt mail](../cli/gt_mail/)	 - Agent messaging system
* [gt maintain](../cli/gt_maintain/)	 - Run full Dolt maintenance (reap + flatten + gc)
* [gt mayor](../cli/gt_mayor/)	 - Manage the Mayor (Chief of Staff for cross-rig coordination)
* [gt metrics](../cli/gt_metrics/)	 - Show command usage statistics
* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands
* [gt mq](../cli/gt_mq/)	 - Merge queue operations
* [gt namepool](../cli/gt_namepool/)	 - Manage polecat name pools
* [gt notify](../cli/gt_notify/)	 - Set notification level
* [gt nudge](../cli/gt_nudge/)	 - Send a synchronous message to any Gas Town worker
* [gt orphans](../cli/gt_orphans/)	 - Find lost polecat work
* [gt override](../cli/gt_override/)	 - Manage role prompt overrides
* [gt patrol](../cli/gt_patrol/)	 - Patrol digest management
* [gt peek](../cli/gt_peek/)	 - View recent output from a polecat or crew session
* [gt plugin](../cli/gt_plugin/)	 - Plugin management
* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)
* [gt prime](../cli/gt_prime/)	 - Output role context for current directory
* [gt prune-branches](../cli/gt_prune-branches/)	 - Remove stale local polecat tracking branches
* [gt quota](../cli/gt_quota/)	 - Manage account quota rotation
* [gt ready](../cli/gt_ready/)	 - Show work ready across town
* [gt refinery](../cli/gt_refinery/)	 - Manage the Refinery (merge queue processor)
* [gt release](../cli/gt_release/)	 - Release stuck in_progress issues back to pending
* [gt resume](../cli/gt_resume/)	 - Check for handoff messages
* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace
* [gt role](../cli/gt_role/)	 - Show or manage agent role
* [gt scheduler](../cli/gt_scheduler/)	 - Manage dispatch scheduler
* [gt seance](../cli/gt_seance/)	 - Talk to your predecessor sessions
* [gt session](../cli/gt_session/)	 - Manage polecat sessions
* [gt shell](../cli/gt_shell/)	 - Manage shell integration
* [gt show](../cli/gt_show/)	 - Show details of a bead
* [gt shutdown](../cli/gt_shutdown/)	 - Shutdown Gas Town with cleanup
* [gt signal](../cli/gt_signal/)	 - Claude Code hook signal handlers
* [gt sling](../cli/gt_sling/)	 - Assign work to an agent (THE unified work dispatch command)
* [gt stale](../cli/gt_stale/)	 - Check if the gt binary is stale
* [gt start](../cli/gt_start/)	 - Start Gas Town or a crew workspace
* [gt status](../cli/gt_status/)	 - Show overall town status
* [gt synthesis](../cli/gt_synthesis/)	 - Manage convoy synthesis steps
* [gt tap](../cli/gt_tap/)	 - Claude Code hook handlers
* [gt thanks](../cli/gt_thanks/)	 - Thank the human contributors to Gas Town
* [gt theme](../cli/gt_theme/)	 - View or set tmux theme for the current rig
* [gt town](../cli/gt_town/)	 - Town-level operations
* [gt trail](../cli/gt_trail/)	 - Show recent agent activity
* [gt uninstall](../cli/gt_uninstall/)	 - Remove Gas Town from the system
* [gt unsling](../cli/gt_unsling/)	 - Remove work from an agent's hook
* [gt up](../cli/gt_up/)	 - Bring up all Gas Town services
* [gt upgrade](../cli/gt_upgrade/)	 - Run post-install migration and sync workspace state
* [gt version](../cli/gt_version/)	 - Print version information
* [gt vitals](../cli/gt_vitals/)	 - Show unified health dashboard
* [gt warrant](../cli/gt_warrant/)	 - Manage death warrants for stuck agents
* [gt whoami](../cli/gt_whoami/)	 - Show current identity for mail commands
* [gt witness](../cli/gt_witness/)	 - Manage the Witness (per-rig polecat health monitor)
* [gt wl](../cli/gt_wl/)	 - Wasteland federation commands
* [gt worktree](../cli/gt_worktree/)	 - Create worktree in another rig for cross-rig work

