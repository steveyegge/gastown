---
title: "DOCS/CLI/GT DEACON STATUS"
---

## gt deacon status

Check Deacon session status

### Synopsis

Check if the Deacon tmux session is currently running.

Shows whether the Deacon has an active tmux session and reports
its session name. The Deacon is the town-level watchdog that
receives heartbeats from the daemon.

Examples:
  gt deacon status

```
gt deacon status [flags]
```

### Options

```
  -h, --help   help for status
      --json   Output as JSON
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

