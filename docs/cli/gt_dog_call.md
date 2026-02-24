---
title: "DOCS/CLI/GT DOG CALL"
---

## gt dog call

Wake idle dog(s) for work

### Synopsis

Wake an idle dog to prepare for work.

With a name, wakes the specific dog.
With --all, wakes all idle dogs.
Without arguments, wakes one idle dog (if available).

This updates the dog's last-active timestamp and can trigger
session creation for the dog's worktrees.

Examples:
  gt dog call alpha
  gt dog call --all
  gt dog call

```
gt dog call [name] [flags]
```

### Options

```
      --all    Wake all idle dogs
  -h, --help   help for call
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

