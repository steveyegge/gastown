---
title: "DOCS/CLI/GT MAIL QUEUE CREATE"
---

## gt mail queue create

Create a new queue

### Synopsis

Create a new beads-native mail queue.

The --claimers flag specifies a pattern for who can claim messages from this queue.
Patterns support wildcards: 'gastown/polecats/*' matches any polecat in gastown rig.

Examples:
  gt mail queue create work --claimers 'gastown/polecats/*'
  gt mail queue create dispatch --claimers 'gastown/crew/*'
  gt mail queue create urgent --claimers '*'

```
gt mail queue create <name> [flags]
```

### Options

```
      --claimers string   Pattern for who can claim from this queue (required)
  -h, --help              help for create
```

### SEE ALSO

* [gt mail queue](../cli/gt_mail_queue/)	 - Manage mail queues

