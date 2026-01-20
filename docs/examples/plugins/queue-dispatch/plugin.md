+++
name = "queue-dispatch"
background = true  # Run via Bash run_in_background to survive nudges

[gate]
type = "cooldown"
duration = "30s"
+++

# Queue Dispatch

Dispatch queued beads to polecats.

```bash
gt queue run
```

This checks the work queue and spawns polecats for any queued beads,
respecting capacity limits from town/rig config.

If the queue is empty, this is a no-op.
