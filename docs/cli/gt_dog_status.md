---
title: "GT DOG STATUS"
---

## gt dog status

Show detailed dog status

### Synopsis

Show detailed status for a specific dog or summary for all dogs.

With a name, shows detailed info including:
  - State (idle/working)
  - Current work assignment
  - Worktree paths per rig
  - Last active timestamp

Without a name, shows pack summary:
  - Total dogs
  - Idle/working counts
  - Pack health

Examples:
  gt dog status alpha
  gt dog status
  gt dog status --json

```
gt dog status [name] [flags]
```

### Options

```
  -h, --help   help for status
      --json   Output as JSON
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

