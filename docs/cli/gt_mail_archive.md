---
title: "DOCS/CLI/GT MAIL ARCHIVE"
---

## gt mail archive

Archive messages

### Synopsis

Archive one or more messages.

Removes the messages from your inbox by closing them in beads.

Use --stale to archive messages sent before your current session started.

Examples:
	gt mail archive hq-abc123
	gt mail archive hq-abc123 hq-def456 hq-ghi789
	gt mail archive --stale
	gt mail archive --stale --dry-run

```
gt mail archive [message-id...] [flags]
```

### Options

```
  -n, --dry-run   Show what would be archived without archiving
  -h, --help      help for archive
      --stale     Archive messages sent before session start
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

