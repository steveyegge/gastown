---
title: "DOCS/CLI/GT DOG DONE"
---

## gt dog done

Mark dog as done and return to idle

### Synopsis

Mark a dog as done with its current work and return to idle state.

Dogs should call this when they complete their work assignment.
This clears the work field and sets state to idle, making the dog
available for new work.

Without a name argument, auto-detects the current dog from the working
directory (must be run from within a dog's worktree).

Examples:
  gt dog done         # Auto-detect from cwd
  gt dog done alpha   # Explicit name

```
gt dog done [name] [flags]
```

### Options

```
  -h, --help   help for done
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

