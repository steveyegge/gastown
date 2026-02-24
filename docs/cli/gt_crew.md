---
title: "DOCS/CLI/GT CREW"
---

## gt crew

Manage crew workers (persistent workspaces for humans)

### Synopsis

Manage crew workers - persistent workspaces for human developers.

CREW VS POLECATS:
  Polecats: Ephemeral sessions. Witness-managed. Auto-nuked after work.
  Crew:     Persistent. User-managed. Stays until you remove it.

Crew workers are full git clones (not worktrees) for human developers
who want persistent context and control over their workspace lifecycle.
Use crew workers for exploratory work, long-running tasks, or when you
want to keep uncommitted changes around.

Features:
  - Gas Town integrated: Mail, nudge, handoff all work
  - Recognizable names: dave, emma, fred (not ephemeral pool names)
  - Tmux optional: Can work in terminal directly without tmux session

Commands:
  gt crew start <name>     Start session (creates workspace if needed)
  gt crew stop <name>      Stop session(s)
  gt crew add <name>       Create workspace without starting
  gt crew list             List workspaces with status
  gt crew at <name>        Attach to session
  gt crew remove <name>    Remove workspace
  gt crew refresh <name>   Context cycle with handoff mail
  gt crew restart <name>   Kill and restart session fresh

```
gt crew [flags]
```

### Options

```
  -h, --help   help for crew
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt crew add](../cli/gt_crew_add/)	 - Create a new crew workspace
* [gt crew at](../cli/gt_crew_at/)	 - Attach to crew workspace session
* [gt crew list](../cli/gt_crew_list/)	 - List crew workspaces with status
* [gt crew pristine](../cli/gt_crew_pristine/)	 - Sync crew workspaces with remote
* [gt crew refresh](../cli/gt_crew_refresh/)	 - Context cycling with mail-to-self handoff
* [gt crew remove](../cli/gt_crew_remove/)	 - Remove crew workspace(s)
* [gt crew rename](../cli/gt_crew_rename/)	 - Rename a crew workspace
* [gt crew restart](../cli/gt_crew_restart/)	 - Kill and restart crew workspace session(s)
* [gt crew start](../cli/gt_crew_start/)	 - Start crew worker(s) in a rig
* [gt crew status](../cli/gt_crew_status/)	 - Show detailed workspace status
* [gt crew stop](../cli/gt_crew_stop/)	 - Stop crew workspace session(s)

