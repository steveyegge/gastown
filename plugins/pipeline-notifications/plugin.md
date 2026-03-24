+++
name = "pipeline-notifications"
description = "Subscribe to ntfy.sh/beacon-ci stream for real-time pipeline pass/fail/revert notifications"
version = 1

[gate]
type = "cooldown"
duration = "5m"

[tracking]
labels = ["plugin:pipeline-notifications", "category:ci-monitoring"]
digest = true

[execution]
type = "script"
timeout = "2m"
notify_on_failure = true
severity = "medium"
+++

# Pipeline Notifications

Subscribes to the `beacon-ci` ntfy.sh topic for real-time CI pipeline
notifications. Event-driven, zero polling delay.

The `run.sh` script checks if the watcher is already running, starts it
if not, and exits. The watcher is a persistent `curl` streaming connection
to `ntfy.sh/beacon-ci/json` that parses each event and mails the mayor.
