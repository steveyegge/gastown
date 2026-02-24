---
title: "GT CALLBACKS"
---

## gt callbacks

Handle agent callbacks

### Synopsis

Handle callbacks from agents during Deacon patrol.

Callbacks are messages sent to the Mayor from:
- Witnesses reporting polecat status
- Refineries reporting merge results
- Polecats requesting help or escalation
- External triggers (webhooks, timers)

This command processes the Mayor's inbox and handles each message
appropriately, routing to other agents or updating state as needed.

### Options

```
  -h, --help   help for callbacks
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt callbacks process](../cli/gt_callbacks_process/)	 - Process pending callbacks

