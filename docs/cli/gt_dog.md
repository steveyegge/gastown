---
title: "DOCS/CLI/GT DOG"
---

## gt dog

Manage dogs (cross-rig infrastructure workers)

### Synopsis

Manage dogs - reusable workers for infrastructure and cleanup.

CATS VS DOGS:
  Polecats (cats) build features. One rig. Ephemeral sessions (one task, then nuked).
  Dogs clean up messes. Cross-rig. Reusable (multiple tasks, eventually recycled).

Dogs are managed by the Deacon for town-level work:
  - Infrastructure tasks (rebuilding, syncing, migrations)
  - Cleanup operations (orphan branches, stale files)
  - Cross-rig work that spans multiple projects

Each dog has worktrees into every configured rig, enabling cross-project
operations. Dogs return to idle state after completing work (unlike cats).

The kennel is at ~/gt/deacon/dogs/. The Deacon dispatches work to dogs.

### Options

```
  -h, --help   help for dog
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt dog add](../cli/gt_dog_add/)	 - Create a new dog in the kennel
* [gt dog call](../cli/gt_dog_call/)	 - Wake idle dog(s) for work
* [gt dog clear](../cli/gt_dog_clear/)	 - Reset a stuck dog to idle state
* [gt dog dispatch](../cli/gt_dog_dispatch/)	 - Dispatch plugin execution to a dog
* [gt dog done](../cli/gt_dog_done/)	 - Mark dog as done and return to idle
* [gt dog health-check](../cli/gt_dog_health-check/)	 - Check dog health (zombies, hung, orphans)
* [gt dog list](../cli/gt_dog_list/)	 - List all dogs in the kennel
* [gt dog remove](../cli/gt_dog_remove/)	 - Remove dogs from the kennel
* [gt dog status](../cli/gt_dog_status/)	 - Show detailed dog status

