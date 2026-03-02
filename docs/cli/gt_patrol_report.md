---
title: "GT PATROL REPORT"
---

## gt patrol report

Close patrol cycle with summary and start next cycle

### Synopsis

Close the current patrol cycle, recording a summary of observations,
then automatically start a new patrol cycle.

This replaces the old squash+new pattern with a single command that:
  1. Closes the current patrol root wisp with the summary
  2. Creates a new patrol wisp for the next cycle

The summary is stored on the patrol root wisp for audit purposes.

Examples:
  gt patrol report --summary "All clear, no issues"
  gt patrol report --summary "Dolt latency elevated, filed escalation"

```
gt patrol report [flags]
```

### Options

```
  -h, --help             help for report
      --summary string   Brief summary of patrol observations (required)
```

### SEE ALSO

* [gt patrol](../cli/gt_patrol/)	 - Patrol digest management

