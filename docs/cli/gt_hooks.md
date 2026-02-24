---
title: "DOCS/CLI/GT HOOKS"
---

## gt hooks

Centralized hook management for Gas Town

### Synopsis

Manage Claude Code hooks across the Gas Town workspace.

Provides centralized hook configuration with a base config and
per-role/per-rig overrides. Changes are propagated to all workers
via the sync command.

Subcommands:
  base       Edit the shared base hook config
  override   Edit overrides for a role or rig
  sync       Regenerate all .claude/settings.json files
  diff       Show what sync would change
  list       Show all managed settings.json locations
  scan       Scan workspace for existing hooks
  registry   List hooks from the registry
  install    Install a hook from the registry

Config structure:
  Base:      ~/.gt/hooks-base.json
  Overrides: ~/.gt/hooks-overrides/<target>.json

Merge strategy: base → role → rig+role (more specific wins)

Examples:
  gt hooks sync           # Regenerate all settings.json files
  gt hooks diff           # Preview what sync would change
  gt hooks base           # Edit the shared base config
  gt hooks override crew  # Edit overrides for all crew workers
  gt hooks list           # Show managed locations and sync status

```
gt hooks [flags]
```

### Options

```
  -h, --help   help for hooks
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt hooks base](../cli/gt_hooks_base/)	 - Edit the shared base hook config
* [gt hooks diff](../cli/gt_hooks_diff/)	 - Show what sync would change
* [gt hooks init](../cli/gt_hooks_init/)	 - Bootstrap base config from existing settings.json files
* [gt hooks install](../cli/gt_hooks_install/)	 - Install a hook from the registry
* [gt hooks list](../cli/gt_hooks_list/)	 - Show all managed settings.json locations
* [gt hooks override](../cli/gt_hooks_override/)	 - Edit overrides for a role or rig
* [gt hooks registry](../cli/gt_hooks_registry/)	 - List available hooks from the registry
* [gt hooks scan](../cli/gt_hooks_scan/)	 - Scan workspace for existing Claude Code hooks
* [gt hooks sync](../cli/gt_hooks_sync/)	 - Regenerate all .claude/settings.json files

