---
title: "DOCS/CLI/GT MAIL REPLY"
---

## gt mail reply

Reply to a message

### Synopsis

Reply to a specific message.

This is a convenience command that automatically:
- Sets the reply-to field to the original message
- Prefixes the subject with "Re: " (if not already present)
- Sends to the original sender

The message body can be provided as a positional argument or via -m flag.

Examples:
  gt mail reply msg-abc123 "Thanks, working on it now"
  gt mail reply msg-abc123 -m "Thanks, working on it now"
  gt mail reply msg-abc123 -s "Custom subject" -m "Reply body"

```
gt mail reply <message-id> [message] [flags]
```

### Options

```
      --body string      Reply message body (alias for --message)
  -h, --help             help for reply
  -m, --message string   Reply message body
  -s, --subject string   Override reply subject (default: Re: <original>)
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

