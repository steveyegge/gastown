---
title: "DOCS/CLI/GT AUDIT"
---

## gt audit

Query work history by actor

### Synopsis

Query provenance data across git commits, beads, and events.

Shows a unified timeline of work performed by an actor including:
  - Git commits authored by the actor
  - Beads (issues) created by the actor
  - Beads closed by the actor (via assignee)
  - Town log events (spawn, done, handoff, etc.)
  - Activity feed events

Examples:
  gt audit --actor=greenplace/crew/joe       # Show all work by joe
  gt audit --actor=greenplace/polecats/toast # Show polecat toast's work
  gt audit --actor=mayor                  # Show mayor's activity
  gt audit --since=24h                    # Show all activity in last 24h
  gt audit --actor=joe --since=1h         # Combined filters
  gt audit --json                         # Output as JSON

```
gt audit [flags]
```

### Options

```
      --actor string   Filter by actor (agent address or partial match)
  -h, --help           help for audit
      --json           Output as JSON
  -n, --limit int      Maximum number of entries to show (default 50)
      --since string   Show events since duration (e.g., 1h, 24h, 7d)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

