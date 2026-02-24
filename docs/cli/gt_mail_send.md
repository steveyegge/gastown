---
title: "GT MAIL SEND"
---

## gt mail send

Send a message

### Synopsis

Send a message to an agent.

Addresses:
  mayor/           - Send to Mayor
  <rig>/refinery   - Send to a rig's Refinery
  <rig>/<polecat>  - Send to a specific polecat
  <rig>/           - Broadcast to a rig
  list:<name>      - Send to a mailing list (fans out to all members)

Mailing lists are defined in ~/gt/config/messaging.json and allow
sending to multiple recipients at once. Each recipient gets their
own copy of the message.

Message types:
  task          - Required processing
  scavenge      - Optional first-come work
  notification  - Informational (default)
  reply         - Response to message

Priority levels:
  0 - urgent/critical
  1 - high
  2 - normal (default)
  3 - low
  4 - backlog

Use --urgent as shortcut for --priority 0.

Examples:
  gt mail send greenplace/Toast -s "Status check" -m "How's that bug fix going?"
  gt mail send mayor/ -s "Work complete" -m "Finished gt-abc"
  gt mail send gastown/ -s "All hands" -m "Swarm starting" --notify
  gt mail send greenplace/Toast -s "Task" -m "Fix bug" --type task --priority 1
  gt mail send greenplace/Toast -s "Urgent" -m "Help!" --urgent
  gt mail send mayor/ -s "Re: Status" -m "Done" --reply-to msg-abc123
  gt mail send --self -s "Handoff" -m "Context for next session"
  gt mail send greenplace/Toast -s "Update" -m "Progress report" --cc overseer
  gt mail send list:oncall -s "Alert" -m "System down"

  # Read body from stdin (avoids shell quoting issues):
  gt mail send mayor/ -s "Update" --stdin <<'BODY'
  Message with 'quotes' and "quotes" and $variables.
  BODY

```
gt mail send <address> [flags]
```

### Options

```
      --body string       Alias for --message
      --cc stringArray    CC recipients (can be used multiple times)
  -h, --help              help for send
  -m, --message string    Message body
      --no-notify         Suppress auto-nudge notification to recipient
  -n, --notify            Bump priority to high (notification is automatic; use --no-notify to suppress)
      --permanent         Send as permanent (not ephemeral, synced to remote)
      --pinned            Pin message (for handoff context that persists)
      --priority int      Message priority (0=urgent, 1=high, 2=normal, 3=low, 4=backlog) (default 2)
      --reply-to string   Message ID this is replying to
      --self              Send to self (auto-detect from cwd)
      --stdin             Read message body from stdin (avoids shell quoting issues)
  -s, --subject string    Message subject (required)
      --type string       Message type (task, scavenge, notification, reply) (default "notification")
      --urgent            Set priority=0 (urgent)
      --wisp              Send as wisp (ephemeral, default) (default true)
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

