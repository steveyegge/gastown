---
title: "DOCS/CLI/GT WARRANT EXECUTE"
---

## gt warrant execute

Execute a warrant (terminate agent)

### Synopsis

Execute a pending warrant for the specified target.

This will:
1. Find the warrant for the target
2. Terminate the agent's tmux session (if exists)
3. Mark the warrant as executed

Use --force to execute even if no warrant exists.

Examples:
  gt warrant execute gastown/polecats/alpha
  gt warrant execute deacon/dogs/bravo --force

```
gt warrant execute <target> [flags]
```

### Options

```
  -f, --force   Execute even without a warrant
  -h, --help    help for execute
```

### SEE ALSO

* [gt warrant](../cli/gt_warrant/)	 - Manage death warrants for stuck agents

