---
title: "DOCS/CLI/GT SESSION"
---

## gt session

Manage polecat sessions

### Synopsis

Manage tmux sessions for polecats.

Sessions are tmux sessions running Claude for each polecat.
Use the subcommands to start, stop, attach, and monitor sessions.

TIP: To send messages to a running session, use 'gt nudge' (not 'session inject').
The nudge command uses reliable delivery that works correctly with Claude Code.

```
gt session [flags]
```

### Options

```
  -h, --help   help for session
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt session at](../cli/gt_session_at/)	 - Attach to a running session
* [gt session capture](../cli/gt_session_capture/)	 - Capture recent session output
* [gt session check](../cli/gt_session_check/)	 - Check session health for polecats
* [gt session inject](../cli/gt_session_inject/)	 - Send message to session (prefer 'gt nudge')
* [gt session list](../cli/gt_session_list/)	 - List all sessions
* [gt session restart](../cli/gt_session_restart/)	 - Restart a polecat session
* [gt session start](../cli/gt_session_start/)	 - Start a polecat session
* [gt session status](../cli/gt_session_status/)	 - Show session status details
* [gt session stop](../cli/gt_session_stop/)	 - Stop a polecat session

