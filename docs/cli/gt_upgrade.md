---
title: "GT UPGRADE"
---

## gt upgrade

Run post-install migration and sync workspace state

### Synopsis

Run post-binary-install migrations to bring the workspace up to date.

This is the user-facing entry point for upgrading Gas Town after installing
a new binary. It orchestrates all migration steps in the right order:

  1. Structural checks   Run gt doctor --fix to repair workspace structure
  2. CLAUDE.md sync       Update town root CLAUDE.md from embedded template
  3. Daemon defaults      Ensure daemon.json has lifecycle defaults
  4. Hooks sync           Regenerate settings.json from hook registry
  5. Formula update       Update formulas from embedded copies

Each step reports what changed. Use --dry-run to preview without modifying.

Examples:
  gt upgrade                  # Run all migration steps
  gt upgrade --dry-run        # Show what would change
  gt upgrade --verbose        # Show detailed output
  gt upgrade --no-start       # Suppress starting daemon during doctor fix

```
gt upgrade [flags]
```

### Options

```
      --dry-run    Show what would change without modifying anything
  -h, --help       help for upgrade
      --no-start   Suppress starting daemon/agents during doctor fix
  -v, --verbose    Show detailed output
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

