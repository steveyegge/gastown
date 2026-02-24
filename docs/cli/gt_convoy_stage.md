---
title: "DOCS/CLI/GT CONVOY STAGE"
---

## gt convoy stage

Stage a convoy: analyze dependencies, compute waves, create staged convoy

### Synopsis

Analyze bead dependencies, compute execution waves, and create a staged convoy.

Three input forms:
  gt convoy stage <epic-id>           Walk epic's children, analyze all descendants
  gt convoy stage <task1> <task2>...  Analyze exactly the given tasks
  gt convoy stage <convoy-id>         Re-analyze an existing convoy's tracked beads

The staged convoy can later be launched with 'gt convoy launch'.

```
gt convoy stage <epic-id | task-id... | convoy-id> [flags]
```

### Options

```
  -h, --help     help for stage
      --json     Output machine-readable JSON
      --launch   Launch the convoy immediately after staging (transition to open)
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

