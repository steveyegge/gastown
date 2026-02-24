---
title: "DOCS/CLI/GT SIGNAL"
---

## gt signal

Claude Code hook signal handlers

### Synopsis

Signal handlers for Claude Code hooks.

These commands are designed to be called by Claude Code's hooks system,
not directly by users. They output JSON that Claude Code interprets.

Subcommands:
  stop   Called by the Stop hook at turn boundaries. Checks for queued
         work/messages and either blocks (injects work) or allows (agent
         goes idle).

Example hook configuration (.claude/settings.json):
  {
    "hooks": {
      "Stop": [{
        "hooks": [{
          "type": "command",
          "command": "gt signal stop"
        }]
      }]
    }
  }

### Options

```
  -h, --help   help for signal
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt signal stop](../cli/gt_signal_stop/)	 - Stop hook handler — check for queued work at turn boundaries

