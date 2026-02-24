---
title: "GT UP"
---

## gt up

Bring up all Gas Town services

### Synopsis

Start all Gas Town long-lived services.

This is the idempotent "boot" command for Gas Town. It ensures all
infrastructure agents are running:

  • Dolt       - Shared SQL database server for beads
  • Daemon     - Go background process that pokes agents
  • Deacon     - Health orchestrator (monitors Mayor/Witnesses)
  • Mayor      - Global work coordinator
  • Witnesses  - Per-rig polecat managers
  • Refineries - Per-rig merge queue processors

Polecats are NOT started by this command - they are transient workers
spawned on demand by the Mayor or Witnesses.

Use --restore to also start:
  • Crew       - Per rig settings (settings/config.json crew.startup)
  • Polecats   - Those with pinned beads (work attached)

Running 'gt up' multiple times is safe - it only starts services that
aren't already running.

```
gt up [flags]
```

### Options

```
  -h, --help      help for up
      --json      Output as JSON
  -q, --quiet     Only show errors (ignored with --json)
      --restore   Also restore crew (from settings) and polecats (from hooks)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

