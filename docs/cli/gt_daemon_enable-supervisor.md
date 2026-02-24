---
title: "DOCS/CLI/GT DAEMON ENABLE-SUPERVISOR"
---

## gt daemon enable-supervisor

Configure launchd/systemd for daemon auto-restart

### Synopsis

Configure external supervision for the Gas Town daemon.

This command creates and enables a supervisor service (launchd on macOS,
systemd on Linux) that will automatically restart the daemon if it crashes
or terminates. The daemon will also start automatically on login/boot.

Examples:
  gt daemon enable-supervisor    # Configure launchd/systemd

```
gt daemon enable-supervisor [flags]
```

### Options

```
  -h, --help   help for enable-supervisor
```

### SEE ALSO

* [gt daemon](../cli/gt_daemon/)	 - Manage the Gas Town daemon

