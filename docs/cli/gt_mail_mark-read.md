---
title: "DOCS/CLI/GT MAIL MARK-READ"
---

## gt mail mark-read

Mark messages as read without archiving

### Synopsis

Mark one or more messages as read without removing them from inbox.

This adds a 'read' label to the message, which is reflected in the inbox display.
The message remains in your inbox (unlike archive which closes/removes it).

Use case: You've read a message but want to keep it visible in your inbox
for reference or follow-up.

Examples:
  gt mail mark-read hq-abc123
  gt mail mark-read hq-abc123 hq-def456

```
gt mail mark-read <message-id> [message-id...] [flags]
```

### Options

```
  -h, --help   help for mark-read
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

