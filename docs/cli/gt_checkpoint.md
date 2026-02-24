---
title: "GT CHECKPOINT"
---

## gt checkpoint

Manage session checkpoints for crash recovery

### Synopsis

Manage checkpoints for polecat session crash recovery.

Checkpoints capture the current work state so that if a session crashes,
the next session can resume from where it left off.

Checkpoint data includes:
- Current molecule and step
- Hooked bead
- Modified files list
- Git branch and last commit
- Timestamp

Checkpoints are stored in .polecat-checkpoint.json in the polecat directory.

### Options

```
  -h, --help   help for checkpoint
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt checkpoint clear](../cli/gt_checkpoint_clear/)	 - Clear the checkpoint file
* [gt checkpoint read](../cli/gt_checkpoint_read/)	 - Read and display the current checkpoint
* [gt checkpoint write](../cli/gt_checkpoint_write/)	 - Write a checkpoint of current session state

