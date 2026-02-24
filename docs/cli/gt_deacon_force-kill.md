---
title: "DOCS/CLI/GT DEACON FORCE-KILL"
---

## gt deacon force-kill

Force-kill an unresponsive agent session

### Synopsis

Force-kill an agent session that has been detected as stuck.

This command is used by the Deacon when an agent fails consecutive health checks.
It performs the force-kill protocol:

1. Log the intervention (send mail to agent)
2. Kill the tmux session
3. Update agent bead state to "killed"
4. Notify mayor (optional, for visibility)

After force-kill, the agent is 'asleep'. Normal wake mechanisms apply:
- gt rig boot restarts it
- Or stays asleep until next activity trigger

This respects the cooldown period - won't kill if recently killed.

Examples:
  gt deacon force-kill gastown/polecats/max
  gt deacon force-kill gastown/witness --reason="unresponsive for 90s"

```
gt deacon force-kill <agent> [flags]
```

### Options

```
  -h, --help            help for force-kill
      --reason string   Reason for force-kill (included in notifications)
      --skip-notify     Skip sending notification mail to mayor
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

