---
title: "GT DAEMON"
---

## gt daemon

Manage the Gas Town daemon

### Synopsis

Manage the Gas Town background daemon.

The daemon is a simple Go process that:
- Pokes agents periodically (heartbeat)
- Processes lifecycle requests (cycle, restart, shutdown)
- Restarts sessions when agents request cycling

The daemon is a "dumb scheduler" - all intelligence is in agents.

```
gt daemon [flags]
```

### Options

```
  -h, --help   help for daemon
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt daemon clear-backoff](../cli/gt_daemon_clear-backoff/)	 - Clear crash loop backoff for an agent
* [gt daemon enable-supervisor](../cli/gt_daemon_enable-supervisor/)	 - Configure launchd/systemd for daemon auto-restart
* [gt daemon logs](../cli/gt_daemon_logs/)	 - View daemon logs
* [gt daemon rotate-logs](../cli/gt_daemon_rotate-logs/)	 - Rotate daemon log files
* [gt daemon start](../cli/gt_daemon_start/)	 - Start the daemon
* [gt daemon status](../cli/gt_daemon_status/)	 - Show daemon status
* [gt daemon stop](../cli/gt_daemon_stop/)	 - Stop the daemon

