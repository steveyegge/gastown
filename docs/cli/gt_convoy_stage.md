---
title: "GT CONVOY STAGE"
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

Examples:
  gt convoy stage hq-epic-abc         # Stage from epic
  gt convoy stage gt-task1 gt-task2   # Stage specific tasks
  gt convoy stage hq-cv-staged        # Re-stage existing convoy

```
gt convoy stage <epic-id | task-id... | convoy-id> [flags]
```

### Options

```
  -h, --help           help for stage
      --json           Output machine-readable JSON
      --launch         Launch the convoy immediately after staging (transition to open)
      --title string   Human-readable title for the convoy (default: derived from epic title or auto-generated)
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

