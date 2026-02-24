---
title: "DOCS/CLI/GT MOL PROGRESS"
---

## gt mol progress

Show progress through a molecule's steps

### Synopsis

Show the execution progress of an instantiated molecule.

Given a root issue (the parent of molecule steps), displays:
- Total steps and completion status
- Which steps are done, in-progress, ready, or blocked
- Overall progress percentage

This is useful for the Witness to monitor molecule execution.

Example:
  gt molecule progress gt-abc

```
gt mol progress <root-issue-id> [flags]
```

### Options

```
  -h, --help   help for progress
      --json   Output as JSON
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

