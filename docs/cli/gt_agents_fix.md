---
title: "DOCS/CLI/GT AGENTS FIX"
---

## gt agents fix

Fix identity collisions and clean up stale locks

### Synopsis

Clean up identity collisions and stale locks.

This command:
  1. Removes stale locks (where the PID is dead)
  2. Reports collisions that need manual intervention

For collisions with live processes, you must manually:
  - Kill the duplicate session, OR
  - Decide which agent should own the identity

```
gt agents fix [flags]
```

### Options

```
  -h, --help   help for fix
```

### Options inherited from parent commands

```
  -a, --all   Include polecats in the menu
```

### SEE ALSO

* [gt agents](../cli/gt_agents/)	 - List Gas Town agent sessions

