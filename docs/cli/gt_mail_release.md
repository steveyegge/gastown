---
title: "DOCS/CLI/GT MAIL RELEASE"
---

## gt mail release

Release a claimed queue message

### Synopsis

Release a previously claimed message back to its queue.

SYNTAX:
  gt mail release <message-id>

BEHAVIOR:
1. Find the message by ID
2. Verify caller is the one who claimed it (claimed-by label matches)
3. Remove claimed-by and claimed-at labels
4. Message returns to queue for others to claim

ERROR CASES:
- Message not found
- Message is not a queue message
- Message not claimed
- Caller did not claim this message

Examples:
  gt mail release hq-abc123    # Release a claimed message

```
gt mail release <message-id> [flags]
```

### Options

```
  -h, --help   help for release
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

