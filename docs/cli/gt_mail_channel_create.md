---
title: "DOCS/CLI/GT MAIL CHANNEL CREATE"
---

## gt mail channel create

Create a new channel

### Synopsis

Create a new broadcast channel.

Retention policy:
  --retain-count=N  Keep only last N messages (0 = unlimited)
  --retain-hours=N  Delete messages older than N hours (0 = forever)

```
gt mail channel create <name> [flags]
```

### Options

```
  -h, --help               help for create
      --retain-count int   Number of messages to retain (0 = unlimited)
      --retain-hours int   Hours to retain messages (0 = forever)
```

### SEE ALSO

* [gt mail channel](../cli/gt_mail_channel/)	 - Manage and view beads-native channels

