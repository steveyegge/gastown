---
title: "GT MOL STEP AWAIT-EVENT"
---

## gt mol step await-event

Wait for a file-based event on a named channel

### Synopsis

Wait for event files to appear in ~/gt/events/<channel>/, with optional backoff.

Unlike await-signal (which subscribes to the generic beads activity feed),
await-event watches a dedicated event channel directory for .event files.
Events are emitted via "gt mol step emit-event" or programmatically.

Channels are single-consumer: only one process should watch a given channel
at a time. If multiple consumers watch the same channel with --cleanup,
events may be deleted before all consumers read them.

EVENT FORMAT:
Events are JSON files in ~/gt/events/<channel>/*.event:
  {"type": "...", "channel": "...", "timestamp": "...", "payload": {...}}

BEHAVIOR:
1. Check for already-pending events (return immediately if found)
2. If none, poll the directory until a new .event file appears or timeout
3. On wake, return all pending event file paths and contents
4. With --cleanup, delete processed event files automatically

BACKOFF MODE:
Same as await-signal: base * multiplier^idle_cycles, capped at max.
Idle cycles and backoff-until timestamp tracked on agent bead labels.
If killed and restarted, backoff resumes from the stored backoff-until.

EXIT CODES:
  0 - Event(s) found or timeout
  1 - Error

EXAMPLES:
  # Wait for refinery events with 10min timeout
  gt mol step await-event --channel refinery --timeout 10m

  # Backoff mode with agent bead tracking
  gt mol step await-event --channel refinery --agent-bead VAS-refinery \
    --backoff-base 60s --backoff-mult 2 --backoff-max 10m

  # Auto-cleanup processed events
  gt mol step await-event --channel refinery --cleanup

```
gt mol step await-event [flags]
```

### Options

```
      --agent-bead string     Agent bead ID for tracking idle cycles
      --backoff-base string   Base interval for exponential backoff (e.g., 60s)
      --backoff-max string    Maximum interval cap for backoff (e.g., 10m)
      --backoff-mult int      Multiplier for exponential backoff (default: 2) (default 2)
      --channel string        Event channel name (required, e.g., 'refinery')
      --cleanup               Delete event files after reading them
  -h, --help                  help for await-event
      --json                  Output as JSON
      --quiet                 Suppress output (for scripting)
      --timeout string        Maximum time to wait for event (e.g., 30s, 5m, 10m) (default "60s")
```

### SEE ALSO

* [gt mol step](../cli/gt_mol_step/)	 - Molecule step operations

