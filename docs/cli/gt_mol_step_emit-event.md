---
title: "GT MOL STEP EMIT-EVENT"
---

## gt mol step emit-event

Emit a file-based event on a named channel

### Synopsis

Emit an event file to ~/gt/events/<channel>/ for subscribers to pick up.

This is the Go counterpart to emit-event.sh. Events are JSON files consumed
by await-event subscribers (e.g., the refinery watching for MERGE_READY events).

EVENT FORMAT:
Creates a JSON file at ~/gt/events/<channel>/<timestamp>.event:
  {"type": "...", "channel": "...", "timestamp": "...", "payload": {...}}

EXAMPLES:
  # Emit a MERGE_READY event for the refinery
  gt mol step emit-event --channel refinery --type MERGE_READY \
    --payload polecat=nux --payload branch=polecat/nux/gt-iw7m

  # Emit a PATROL_WAKE event
  gt mol step emit-event --channel refinery --type PATROL_WAKE \
    --payload source=witness --payload queue_depth=3

  # Emit an MQ_SUBMIT event
  gt mol step emit-event --channel refinery --type MQ_SUBMIT \
    --payload branch=feat/new-feature --payload mr_id=bd-42

```
gt mol step emit-event [flags]
```

### Options

```
      --channel string        Event channel name (required, e.g., 'refinery')
  -h, --help                  help for emit-event
      --json                  Output as JSON
      --payload stringArray   Payload key=value pairs (repeatable)
      --type string           Event type (required, e.g., 'MERGE_READY')
```

### SEE ALSO

* [gt mol step](../cli/gt_mol_step/)	 - Molecule step operations

