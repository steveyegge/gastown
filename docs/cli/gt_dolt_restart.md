---
title: "GT DOLT RESTART"
---

## gt dolt restart

Restart the Dolt server (kills imposters)

### Synopsis

Stop the Dolt SQL server, kill any imposter servers on the configured port,
and start the correct server from the configured data directory.

This is the nuclear option for recovering from a hijacked port — when another
process (e.g., bd's embedded Dolt server) has taken over the port with a
different data directory, serving empty/wrong databases.

Steps:
  1. Stop the tracked server (via PID file)
  2. Kill any other dolt sql-server on the configured port (imposters)
  3. Start the correct server from .dolt-data/

```
gt dolt restart [flags]
```

### Options

```
  -h, --help   help for restart
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

