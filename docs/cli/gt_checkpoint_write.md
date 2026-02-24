---
title: "DOCS/CLI/GT CHECKPOINT WRITE"
---

## gt checkpoint write

Write a checkpoint of current session state

### Synopsis

Capture and write the current session state to a checkpoint file.

This is typically called:
- After closing a molecule step
- Periodically during long work sessions
- Before handoff to another session

The checkpoint captures git state, molecule progress, and hooked work.

```
gt checkpoint write [flags]
```

### Options

```
  -h, --help              help for write
      --molecule string   Override molecule ID (auto-detected if not specified)
      --notes string      Add notes to the checkpoint
      --step string       Override step ID (auto-detected if not specified)
```

### SEE ALSO

* [gt checkpoint](../cli/gt_checkpoint/)	 - Manage session checkpoints for crash recovery

