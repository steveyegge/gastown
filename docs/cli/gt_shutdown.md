---
title: "DOCS/CLI/GT SHUTDOWN"
---

## gt shutdown

Shutdown Gas Town with cleanup

### Synopsis

Shutdown Gas Town by stopping agents and cleaning up polecats.

This is the "done for the day" command - it stops everything AND removes
polecat worktrees/branches. For a reversible pause, use 'gt down' instead.

Comparison:
  gt down      - Pause (stop processes, keep worktrees) - reversible
  gt shutdown  - Done (stop + cleanup worktrees) - permanent cleanup

After killing sessions, polecats are cleaned up:
  - Worktrees are removed
  - Polecat branches are deleted
  - Polecats with uncommitted work are SKIPPED (protected)

Shutdown levels (progressively more aggressive):
  (default)       - Stop infrastructure + polecats + cleanup
  --all           - Also stop crew sessions
  --polecats-only - Only stop polecats (leaves infrastructure running)

Use --force or --yes to skip confirmation prompt.
Use --graceful to allow agents time to save state before killing.
Use --nuclear to force cleanup even if polecats have uncommitted work (DANGER).
Use --cleanup-orphans to use a longer grace period for orphan cleanup (default 60s).
Use --cleanup-orphans-grace-secs to set that grace period.

Orphaned Claude processes are always cleaned up after session termination.
By default, a 5-second grace period is used. The --cleanup-orphans flag
extends this to --cleanup-orphans-grace-secs (default 60s) for stubborn processes.

```
gt shutdown [flags]
```

### Options

```
  -a, --all                              Also stop crew sessions (by default, crew is preserved)
      --cleanup-orphans                  Use longer grace period (--cleanup-orphans-grace-secs) for orphan cleanup instead of default 5s
      --cleanup-orphans-grace-secs int   Grace period in seconds between SIGTERM and SIGKILL when cleaning orphans (default 60) (default 60)
  -f, --force                            Skip confirmation prompt (alias for --yes)
  -g, --graceful                         Send ESC to agents and wait for them to handoff before killing
  -h, --help                             help for shutdown
      --nuclear                          Force cleanup even if polecats have uncommitted work (DANGER: may lose work)
      --polecats-only                    Only stop polecats (minimal shutdown)
  -w, --wait int                         Seconds to wait for graceful shutdown (default 30) (default 30)
  -y, --yes                              Skip confirmation prompt
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

