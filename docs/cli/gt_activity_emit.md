---
title: "DOCS/CLI/GT ACTIVITY EMIT"
---

## gt activity emit

Emit an activity event

### Synopsis

Emit an activity event to the Gas Town activity feed.

Supported event types for witness patrol:
  patrol_started   - When witness begins patrol cycle
  polecat_checked  - When witness checks a polecat
  polecat_nudged   - When witness nudges a stuck polecat
  escalation_sent  - When witness escalates to Mayor/Deacon
  patrol_complete  - When patrol cycle finishes

Supported event types for refinery:
  merge_started    - When refinery starts a merge
  merge_complete   - When merge succeeds
  merge_failed     - When merge fails
  queue_processed  - When refinery finishes processing queue

Common options:
  --actor    Who is emitting the event (e.g., greenplace/witness)
  --rig      Which rig the event is about
  --message  Human-readable message

Examples:
  gt activity emit patrol_started --rig greenplace --count 3
  gt activity emit polecat_checked --rig greenplace --polecat Toast --status working --issue gp-xyz
  gt activity emit polecat_nudged --rig greenplace --polecat Toast --reason "idle for 10 minutes"
  gt activity emit escalation_sent --rig greenplace --target Toast --to mayor --reason "unresponsive"
  gt activity emit patrol_complete --rig greenplace --count 3 --message "All polecats healthy"

```
gt activity emit <event-type> [flags]
```

### Options

```
      --actor string     Actor emitting the event (auto-detected if not set)
      --count int        Polecat count (for patrol events)
  -h, --help             help for emit
      --issue string     Issue ID (for polecat_checked)
      --message string   Human-readable message
      --polecat string   Polecat involved (for polecat_checked, polecat_nudged)
      --reason string    Reason for the action
      --rig string       Rig the event is about
      --status string    Status (for polecat_checked: working, idle, stuck)
      --target string    Target of the action (for escalation)
      --to string        Escalation target (for escalation_sent: mayor, deacon)
```

### SEE ALSO

* [gt activity](../cli/gt_activity/)	 - Emit and view activity events

