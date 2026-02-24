---
title: "GT DOLT RECOVER"
---

## gt dolt recover

Detect and recover from Dolt read-only state

### Synopsis

Detect if the Dolt server is in read-only mode and attempt recovery.

When the Dolt server enters read-only mode (e.g., from concurrent write
contention on the storage manifest), all write operations fail. This command:

  1. Probes the server to detect read-only state
  2. Stops the server if read-only
  3. Restarts the server
  4. Verifies recovery with a write probe

If the server is already writable, this is a no-op.

The daemon performs this check automatically every 30 seconds. Use this command
for immediate recovery without waiting for the daemon's health check loop.

```
gt dolt recover [flags]
```

### Options

```
  -h, --help   help for recover
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

