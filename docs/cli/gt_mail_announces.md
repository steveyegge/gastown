---
title: "GT MAIL ANNOUNCES"
---

## gt mail announces

List or read announce channels

### Synopsis

List available announce channels or read messages from a channel.

SYNTAX:
  gt mail announces              # List all announce channels
  gt mail announces <channel>    # Read messages from a channel

Announce channels are bulletin boards defined in ~/gt/config/messaging.json.
Messages are broadcast to readers and persist until retention limit is reached.
Unlike regular mail, announce messages are NOT removed when read.

BEHAVIOR for 'gt mail announces':
- Loads messaging.json
- Lists all announce channel names
- Shows reader patterns and retain_count for each

BEHAVIOR for 'gt mail announces <channel>':
- Validates channel exists
- Queries beads for messages with announce_channel=<channel>
- Displays in reverse chronological order (newest first)
- Does NOT mark as read or remove messages

Examples:
  gt mail announces              # List all channels
  gt mail announces alerts       # Read messages from 'alerts' channel
  gt mail announces --json       # List channels as JSON

```
gt mail announces [channel] [flags]
```

### Options

```
  -h, --help   help for announces
      --json   Output as JSON
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

