---
title: "GT METRICS"
---

## gt metrics

Show command usage statistics

### Synopsis

Reads ~/.gt/cmd-usage.jsonl and reports which gt commands are used,
how often, and by whom. Helps identify dead commands before pruning.

```
gt metrics [flags]
```

### Options

```
      --by-actor    Show breakdown by actor
      --dead        Show commands defined but never invoked
  -h, --help        help for metrics
      --since int   Only show data from last N days
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

