---
title: "DOCS/CLI/GT BROADCAST"
---

## gt broadcast

Send a nudge message to all workers

### Synopsis

Broadcasts a message to all active workers (polecats and crew).

By default, only workers (polecats and crew) receive the message.
Use --all to include infrastructure agents (mayor, deacon, witness, refinery).

The message is sent as a nudge to each worker's Claude Code session.

Examples:
  gt broadcast "Check your mail"
  gt broadcast --rig greenplace "New priority work available"
  gt broadcast --all "System maintenance in 5 minutes"
  gt broadcast --dry-run "Test message"

```
gt broadcast <message> [flags]
```

### Options

```
      --all          Include all agents (mayor, witness, etc.), not just workers
      --dry-run      Show what would be sent without sending
  -h, --help         help for broadcast
      --rig string   Only broadcast to workers in this rig
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

