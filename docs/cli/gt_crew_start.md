---
title: "DOCS/CLI/GT CREW START"
---

## gt crew start

Start crew worker(s) in a rig

### Synopsis

Start crew workers in a rig, creating workspaces if they don't exist.

The rig name can be provided as the first argument, or inferred from the
current directory. If no crew names are specified, starts all crew in the rig.

The crew session starts in the background with Claude running and ready.

Use --resume to resume a previous session instead of starting fresh. This
passes the agent's resume flag (e.g., Claude's --resume) so the session
picks up where it left off, with proper Gas Town metadata set so GC doesn't
kill the session.

Examples:
  gt crew start beads             # Start all crew in beads rig
  gt crew start                   # Start all crew (rig inferred from cwd)
  gt crew start beads grip fang   # Start specific crew in beads rig
  gt crew start gastown joe       # Start joe in gastown rig
  gt crew start beads ace --resume          # Resume ace's most recent session
  gt crew start beads ace --resume abc123   # Resume specific session ID

```
gt crew start [rig] [name...] [flags]
```

### Options

```
      --account string           Claude Code account handle to use
      --agent string             Agent alias to run crew worker with (overrides rig/town default)
      --all                      Start all crew members in the rig
  -h, --help                     help for start
      --resume string[="last"]   Resume a previous session (optionally specify session ID)
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

