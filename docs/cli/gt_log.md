---
title: "DOCS/CLI/GT LOG"
---

## gt log

View town activity log

### Synopsis

View the centralized log of Gas Town agent lifecycle events.

Events logged include:
  spawn   - new agent created
  wake    - agent resumed
  nudge   - message injected into agent
  handoff - agent handed off to fresh session
  done    - agent finished work
  crash   - agent exited unexpectedly
  kill    - agent killed intentionally

Examples:
  gt log                     # Show last 20 events
  gt log -n 50               # Show last 50 events
  gt log --type spawn        # Show only spawn events
  gt log --agent greenplace/    # Show events for gastown rig
  gt log --since 1h          # Show events from last hour
  gt log -f                  # Follow log (like tail -f)

```
gt log [flags]
```

### Options

```
  -a, --agent string   Filter by agent prefix (e.g., gastown/, greenplace/crew/max)
  -f, --follow         Follow log output (like tail -f)
  -h, --help           help for log
      --since string   Show events since duration (e.g., 1h, 30m, 24h)
  -n, --tail int       Number of events to show (default 20)
  -t, --type string    Filter by event type (spawn,wake,nudge,handoff,done,crash,kill)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt log crash](../cli/gt_log_crash/)	 - Record a crash event (called by tmux pane-died hook)

