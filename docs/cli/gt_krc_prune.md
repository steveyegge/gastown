---
title: "GT KRC PRUNE"
---

## gt krc prune

Remove expired events

### Synopsis

Prune events that have exceeded their TTL.

Events are removed from both .events.jsonl and .feed.jsonl.
The operation is atomic (uses temp files and rename).

Use --dry-run to preview what would be pruned without making changes.

```
gt krc prune [flags]
```

### Options

```
      --auto      Daemon mode: only prune if PruneInterval has elapsed
      --dry-run   Preview changes without modifying files
  -h, --help      help for prune
```

### SEE ALSO

* [gt krc](../cli/gt_krc/)	 - Key Record Chronicle - manage ephemeral data TTLs

