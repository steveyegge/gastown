+++
name = "dolt-log-rotate"
description = "Rotate Dolt server log file when it exceeds size threshold"
version = 1

[gate]
type = "cooldown"
duration = "6h"

[tracking]
labels = ["plugin:dolt-log-rotate", "category:maintenance"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "medium"
+++

# Dolt Log Rotate

The Dolt server writes stdout/stderr to `daemon/dolt.log`. This file can
grow to multiple gigabytes and cause disk pressure or slow `gt dolt logs`.

This plugin checks the log size every 6 hours and rotates when it exceeds
100MB (configurable via `GT_DOLT_LOG_MAX_MB`). Keeps 3 compressed rotated
copies.

Rotation is safe while Dolt is running — the server holds an open file
descriptor, so renaming the log and creating a new one works (Unix fd
semantics). However, new log output continues to the old fd until Dolt is
restarted. To redirect output to the new file, the plugin sends SIGHUP
(if supported) or notes that full rotation completes on next Dolt restart.
