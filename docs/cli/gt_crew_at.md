---
title: "GT CREW AT"
---

## gt crew at

Attach to crew workspace session

### Synopsis

Start or attach to a tmux session for a crew workspace.

Creates a new tmux session if none exists, or attaches to existing.
Use --no-tmux to just print the directory path instead.

When run from inside tmux, the session is started but you stay in your
current pane. Use C-b s to switch to the new session.

When run from outside tmux, you are attached to the session (unless
--detached is specified).

Branch Handling:
  By default, the workspace stays on its current branch (a warning is
  shown if not on the default branch). Use --reset to switch to the
  default branch and pull latest changes.

Role Discovery:
  If no name is provided, attempts to detect the crew workspace from the
  current directory. If you're in <rig>/crew/<name>/, it will attach to
  that workspace automatically.

Examples:
  gt crew at dave                 # Attach to dave's session
  gt crew at                      # Auto-detect from cwd
  gt crew at dave --reset         # Reset to default branch first
  gt crew at dave --detached      # Start session without attaching
  gt crew at dave --no-tmux       # Just print path

```
gt crew at [name] [flags]
```

### Options

```
      --account string   Claude Code account handle to use (overrides default)
      --agent string     Agent alias to run crew worker with (overrides rig/town default)
      --debug            Show debug output for troubleshooting
  -d, --detached         Start session without attaching
  -h, --help             help for at
      --no-tmux          Just print directory path
      --reset            Reset workspace to default branch (checkout and pull)
      --rig string       Rig to use
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

