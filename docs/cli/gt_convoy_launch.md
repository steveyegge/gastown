---
title: "DOCS/CLI/GT CONVOY LAUNCH"
---

## gt convoy launch

Launch a staged convoy: transition to open and dispatch Wave 1

### Synopsis

Launch a staged convoy by transitioning its status from staged to open
and dispatching Wave 1 tasks.

For staged convoy-id input: transitions directly and dispatches.
For epic/task input: runs stage + launch in one step.

```
gt convoy launch <convoy-id | epic-id | task-id...> [flags]
```

### Options

```
      --force   Launch even with warnings
  -h, --help    help for launch
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

