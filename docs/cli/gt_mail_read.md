---
title: "DOCS/CLI/GT MAIL READ"
---

## gt mail read

Read a message

### Synopsis

Read a specific message (does not mark as read).

You can specify a message by its ID or by its numeric index from the inbox.
The index corresponds to the number shown in 'gt mail inbox' (1-based).

Examples:
  gt mail read hq-abc123    # Read by message ID
  gt mail read 3            # Read the 3rd message in inbox

Use 'gt mail inbox' to list messages and their IDs.
Use 'gt mail mark-read' to mark messages as read.

```
gt mail read <message-id|index> [flags]
```

### Options

```
  -h, --help   help for read
      --json   Output as JSON
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

