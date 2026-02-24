---
title: "DOCS/CLI/GT KRC CONFIG SET"
---

## gt krc config set

Set TTL for an event type pattern

### Synopsis

Set the TTL for events matching the given pattern.

Patterns support glob-style matching with * (e.g., "patrol_*" matches all patrol events).
Use "default" as the pattern to set the default TTL.

TTL format: 1h, 12h, 1d, 7d, 30d, etc.

```
gt krc config set <pattern> <ttl> [flags]
```

### Options

```
  -h, --help   help for set
```

### SEE ALSO

* [gt krc config](../cli/gt_krc_config/)	 - View or modify TTL configuration

