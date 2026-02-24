---
title: "DOCS/CLI/GT COSTS RECORD"
---

## gt costs record

Record session cost to local log file (called by Stop hook)

### Synopsis

Record the final cost of a session to a local log file.

This command is intended to be called from a Claude Code Stop hook.
It reads token usage from the Claude Code transcript file (~/.claude/projects/...)
and calculates the cost based on model pricing, then appends it to
~/.gt/costs.jsonl. This is a simple append operation that never fails
due to database availability.

Session costs are aggregated daily by 'gt costs digest' into a single
permanent "Cost Report YYYY-MM-DD" bead for audit purposes.

Examples:
  gt costs record --session gt-gastown-toast
  gt costs record --session gt-gastown-toast --work-item gt-abc123

```
gt costs record [flags]
```

### Options

```
  -h, --help               help for record
      --session string     Tmux session name to record
      --work-item string   Work item ID (bead) for attribution
```

### SEE ALSO

* [gt costs](../cli/gt_costs/)	 - Show costs for running Claude sessions

