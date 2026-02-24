---
title: "GT MAIL CLAIM"
---

## gt mail claim

Claim a message from a queue

### Synopsis

Claim the oldest unclaimed message from a work queue.

SYNTAX:
  gt mail claim [queue-name]

BEHAVIOR:
1. If queue specified, claim from that queue
2. If no queue specified, claim from any eligible queue
3. Add claimed-by and claimed-at labels to the message
4. Print claimed message details

ELIGIBILITY:
The caller must match the queue's claim_pattern (stored in the queue bead).
Pattern examples: "*" (anyone), "gastown/polecats/*" (specific rig crew).

Examples:
  gt mail claim work-requests   # Claim from specific queue
  gt mail claim                 # Claim from any eligible queue

```
gt mail claim [queue-name] [flags]
```

### Options

```
  -h, --help   help for claim
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

