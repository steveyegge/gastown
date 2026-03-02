---
title: "GT DOLT KILL-IMPOSTERS"
---

## gt dolt kill-imposters

Kill dolt servers hijacking this workspace's port

### Synopsis

Find and kill any dolt sql-server that holds this workspace's configured
port but serves from a different data directory (an "imposter").

This is safe to run at any time. It only kills servers that are:
  1. Listening on the same port as this workspace's Dolt config
  2. Serving from a data directory OTHER than this workspace's .dolt-data/

It never kills the workspace's own legitimate Dolt server.

Examples:
  gt dolt kill-imposters          # Kill imposters on configured port
  gt dolt kill-imposters --dry-run # Preview without killing

```
gt dolt kill-imposters [flags]
```

### Options

```
      --dry-run   Preview without killing
  -h, --help      help for kill-imposters
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

