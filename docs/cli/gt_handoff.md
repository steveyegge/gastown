---
title: "DOCS/CLI/GT HANDOFF"
---

## gt handoff

Hand off to a fresh session, work continues from hook

### Synopsis

End watch. Hand off to a fresh agent session.

This is the canonical way to end any agent session. It handles all roles:

  - Mayor, Crew, Witness, Refinery, Deacon: Respawns with fresh Claude instance
  - Polecats: Calls 'gt done --status DEFERRED' (Witness handles lifecycle)

When run without arguments, hands off the current session.
When given a bead ID (gt-xxx, hq-xxx), hooks that work first, then restarts.
When given a role name, hands off that role's session (and switches to it).

Examples:
  gt handoff                          # Hand off current session
  gt handoff gt-abc                   # Hook bead, then restart
  gt handoff gt-abc -s "Fix it"       # Hook with context, then restart
  gt handoff -s "Context" -m "Notes"  # Hand off with custom message
  gt handoff -c                       # Collect state into handoff message
  gt handoff crew                     # Hand off crew session
  gt handoff mayor                    # Hand off mayor session

The --collect (-c) flag gathers current state (hooked work, inbox, ready beads,
in-progress items) and includes it in the handoff mail. This provides context
for the next session without manual summarization.

The --cycle flag triggers automatic session cycling (used by PreCompact hooks).
Unlike --auto (state only) or normal handoff (polecat→gt-done redirect), --cycle
always does a full respawn regardless of role. This enables crew workers and
polecats to get a fresh context window when the current one fills up.

Any molecule on the hook will be auto-continued by the new session.
The SessionStart hook runs 'gt prime' to restore context.

```
gt handoff [bead-or-role] [flags]
```

### Options

```
      --auto             Save state only, no session cycling (for PreCompact hooks)
  -c, --collect          Auto-collect state (status, inbox, beads) into handoff message
      --cycle            Auto-cycle session (for PreCompact hooks that want full session replacement)
  -n, --dry-run          Show what would be done without executing
  -h, --help             help for handoff
  -m, --message string   Message body for handoff mail (optional)
      --no-git-check     Skip git workspace cleanliness check
      --reason string    Reason for handoff (e.g., 'compaction', 'idle')
      --stdin            Read message body from stdin (avoids shell quoting issues)
  -s, --subject string   Subject for handoff mail (optional)
  -w, --watch            Switch to new session (for remote handoff) (default true)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

