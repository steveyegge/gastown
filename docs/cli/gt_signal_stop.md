---
title: "GT SIGNAL STOP"
---

## gt signal stop

Stop hook handler — check for queued work at turn boundaries

### Synopsis

Called by Claude Code's Stop hook at every turn boundary.

Checks for queued work or messages for the current agent:
1. Unread mail (high/critical priority first)
2. Slung work (hooked beads assigned to this agent)

If work is found, outputs {"decision":"block","reason":"<message>"} which
prevents the turn from ending and injects the message as new context.

If nothing is queued, outputs {"decision":"approve"} and the agent goes idle.

This command must complete in <500ms as it runs on every turn boundary.
All output goes to stdout as JSON for Claude Code to consume.

```
gt signal stop [flags]
```

### Options

```
  -h, --help   help for stop
```

### SEE ALSO

* [gt signal](../cli/gt_signal/)	 - Claude Code hook signal handlers

