---
title: "GT NUDGE"
---

## gt nudge

Send a synchronous message to any Gas Town worker

### Synopsis

Universal messaging API for Gas Town worker-to-worker communication.

Delivers a message to any worker's Claude Code session: polecats, crew,
witness, refinery, mayor, or deacon.

Delivery modes (--mode):
  immediate  Send directly via tmux send-keys (default). Interrupts in-flight
             work but guarantees immediate delivery.
  queue      Write to a file queue; agent picks up via hook at next turn
             boundary. Zero interruption. Use for non-urgent coordination.
  wait-idle  Wait for agent to become idle (prompt visible), then deliver
             directly. Falls back to queue on timeout. If both idle-wait and
             queue fail, falls back to immediate delivery as a last resort.

Queue and wait-idle modes require the target agent to support hooks
(UserPromptSubmit) for drain. Agents without hook support should use immediate.

The default is immediate for backward compatibility. For non-urgent messages
where you don't want to interrupt the agent's current work, use --mode=queue.

This is the ONLY way to send messages to Claude sessions.
Do not use raw tmux send-keys elsewhere.

Role shortcuts (expand to session names):
  mayor     Maps to gt-mayor
  deacon    Maps to gt-deacon
  witness   Maps to gt-<rig>-witness (uses current rig)
  refinery  Maps to gt-<rig>-refinery (uses current rig)

Channel syntax:
  channel:<name>  Nudges all members of a named channel defined in
                  ~/gt/config/messaging.json under "nudge_channels".
                  Patterns like "gastown/polecats/*" are expanded.

DND (Do Not Disturb):
  If the target has DND enabled (gt dnd on), the nudge is skipped.
  Use --force to override DND and send anyway.

Examples:
  gt nudge greenplace/furiosa "Check your mail and start working"
  gt nudge greenplace/alpha -m "What's your status?"
  gt nudge mayor "Status update requested"
  gt nudge witness "Check polecat health"
  gt nudge deacon session-started
  gt nudge channel:workers "New priority work available"

  # Use --stdin for messages with special characters or formatting:
  gt nudge gastown/alpha --stdin <<'EOF'
  Status update:
  - Task 1: complete
  - Task 2: in progress
  EOF

```
gt nudge <target> [message] [flags]
```

### Options

```
  -f, --force             Send even if target has DND enabled
  -h, --help              help for nudge
      --if-fresh          Only send if caller's tmux session is <60s old (suppresses compaction nudges)
  -m, --message string    Message to send
      --mode string       Delivery mode: immediate (default), queue, or wait-idle (default "immediate")
      --priority string   Queue priority: normal (default) or urgent (default "normal")
      --stdin             Read message from stdin (avoids shell quoting issues)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

