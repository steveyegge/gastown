---
title: "DOCS/CLI/GT COSTS"
---

## gt costs

Show costs for running Claude sessions

### Synopsis

Display costs for Claude Code sessions in Gas Town.

Costs are calculated from Claude Code transcript files at ~/.claude/projects/
by summing token usage from assistant messages and applying model-specific pricing.

Examples:
  gt costs              # Live costs from running sessions
  gt costs --today      # Today's costs from log file (not yet digested)
  gt costs --week       # This week's costs from digest beads + today's log
  gt costs --by-role    # Breakdown by role (polecat, witness, etc.)
  gt costs --by-rig     # Breakdown by rig
  gt costs --json       # Output as JSON
  gt costs -v           # Show debug output for failures

Subcommands:
  gt costs record       # Record session cost to local log file (Stop hook)
  gt costs digest       # Aggregate log entries into daily digest bead (Deacon patrol)

```
gt costs [flags]
```

### Options

```
      --by-rig    Show breakdown by rig
      --by-role   Show breakdown by role
  -h, --help      help for costs
      --json      Output as JSON
      --today     Show today's total from session events
  -v, --verbose   Show debug output for failures
      --week      Show this week's total from session events
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt costs digest](../cli/gt_costs_digest/)	 - Aggregate session cost log entries into a daily digest bead
* [gt costs migrate](../cli/gt_costs_migrate/)	 - Migrate legacy session.ended beads to the new log-file architecture
* [gt costs record](../cli/gt_costs_record/)	 - Record session cost to local log file (called by Stop hook)

