---
title: "DOCS/CLI/GT MAIL QUEUE"
---

## gt mail queue

Manage mail queues

### Synopsis

Manage beads-native mail queues.

Queues provide a way to distribute work to eligible workers.
Messages sent to a queue can be claimed by workers matching the claim pattern.

COMMANDS:
  create    Create a new queue
  show      Show queue details
  list      List all queues
  delete    Delete a queue

Examples:
  gt mail queue create work --claimers 'gastown/polecats/*'
  gt mail queue show work
  gt mail queue list
  gt mail queue delete work

```
gt mail queue [flags]
```

### Options

```
  -h, --help   help for queue
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system
* [gt mail queue create](../cli/gt_mail_queue_create/)	 - Create a new queue
* [gt mail queue delete](../cli/gt_mail_queue_delete/)	 - Delete a queue
* [gt mail queue list](../cli/gt_mail_queue_list/)	 - List all queues
* [gt mail queue show](../cli/gt_mail_queue_show/)	 - Show queue details

