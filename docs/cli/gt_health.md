---
title: "GT HEALTH"
---

## gt health

Show comprehensive system health

### Synopsis

Display a comprehensive health report for the Gas Town data plane.

Sections:
  1. Dolt Server: status, PID, port, latency
  2. Databases: per-DB counts of issues, wisps, commits
  3. Pollution: scan for known test/garbage patterns
  4. Backups: Dolt filesystem and JSONL git freshness
  5. Processes: zombie dolt servers
  6. Orphan DBs: databases not referenced by any rig

Use --json for machine-readable output.

```
gt health [flags]
```

### Options

```
  -h, --help   help for health
      --json   Output as JSON
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

