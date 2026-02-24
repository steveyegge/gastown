---
title: "DOCS/CLI/GT MAIL INBOX"
---

## gt mail inbox

Check inbox

### Synopsis

Check messages in an inbox.

If no address is specified, shows the current context's inbox.
Use --identity for polecats to explicitly specify their identity.

By default, shows all messages. Use --unread to filter to unread only,
or --all to explicitly show all messages (read and unread).

Examples:
  gt mail inbox                       # Current context (auto-detected)
  gt mail inbox --all                 # Explicitly show all messages
  gt mail inbox --unread              # Show only unread messages
  gt mail inbox mayor/                # Mayor's inbox
  gt mail inbox greenplace/Toast         # Polecat's inbox
  gt mail inbox --identity greenplace/Toast  # Explicit polecat identity

```
gt mail inbox [address] [flags]
```

### Options

```
      --address string    Alias for --identity
  -a, --all               Show all messages (read and unread)
  -h, --help              help for inbox
      --identity string   Explicit identity for inbox (e.g., greenplace/Toast)
      --json              Output as JSON
  -u, --unread            Show only unread messages
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

