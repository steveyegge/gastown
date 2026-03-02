---
title: "GT MOL AWAIT-SIGNAL"
---

## gt mol await-signal

Wait for activity feed signal with timeout (alias: gt mol step await-signal)

### Synopsis

Wait for any activity on the events feed, with optional backoff.

This command is the primary wake mechanism for patrol agents. It tails
~/gt/.events.jsonl and returns immediately when a new event is appended
(indicating Gas Town activity such as slings, nudges, mail, spawns, etc.).

If no activity occurs within the timeout, the command returns with exit code 0
but sets the AWAIT_SIGNAL_REASON environment variable to "timeout".

The timeout can be specified directly or via backoff configuration for
exponential wait patterns.

BACKOFF MODE:
When backoff parameters are provided, the effective timeout is calculated as:
  min(base * multiplier^idle_cycles, max)

The idle_cycles value is read from the agent bead's "idle" label, enabling
exponential backoff that persists across invocations. When a signal is
received, the caller should reset idle:0 on the agent bead.

EXIT CODES:
  0 - Signal received or timeout (check output for which)
  1 - Error opening events file

EXAMPLES:
  # Simple wait with 60s timeout (canonical form)
  gt mol step await-signal --timeout 60s

  # Short form (alias)
  gt mol await-signal --timeout 60s

  # Backoff mode with agent bead tracking:
  gt mol await-signal --agent-bead gt-gastown-witness \
    --backoff-base 30s --backoff-mult 2 --backoff-max 5m

  # On timeout, the agent bead's idle:N label is auto-incremented
  # On signal, caller should reset: gt agent state gt-gastown-witness --set idle=0

  # Quiet mode (no output, for scripting)
  gt mol await-signal --timeout 30s --quiet

```
gt mol await-signal [flags]
```

### Options

```
      --agent-bead string     Agent bead ID for tracking idle cycles (reads/writes idle:N label)
      --backoff-base string   Base interval for exponential backoff (e.g., 30s)
      --backoff-max string    Maximum interval cap for backoff (e.g., 10m)
      --backoff-mult int      Multiplier for exponential backoff (default: 2) (default 2)
  -h, --help                  help for await-signal
      --json                  Output as JSON
      --quiet                 Suppress output (for scripting)
      --timeout string        Maximum time to wait for signal (e.g., 30s, 5m) (default "60s")
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

