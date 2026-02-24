---
title: "GT SEANCE"
---

## gt seance

Talk to your predecessor sessions

### Synopsis

Seance lets you literally talk to predecessor sessions.

"Where did you put the stuff you left for me?" - The #1 handoff question.

Instead of parsing logs, seance spawns a Claude subprocess that resumes
a predecessor session with full context. You can ask questions directly:
  - "Why did you make this decision?"
  - "Where were you stuck?"
  - "What did you try that didn't work?"

DISCOVERY:
  gt seance                     # List recent sessions from events
  gt seance --role crew         # Filter by role type
  gt seance --rig gastown       # Filter by rig
  gt seance --recent 10         # Last N sessions

THE SEANCE (talk to predecessor):
  gt seance --talk <session-id>              # Interactive conversation
  gt seance --talk <id> -p "Where is X?"     # One-shot question

The --talk flag spawns: claude --fork-session --resume <id>
This loads the predecessor's full context without modifying their session.

Sessions are discovered from:
  1. Events emitted by SessionStart hooks (~/gt/.events.jsonl)
  2. The [GAS TOWN] beacon makes sessions searchable in /resume

```
gt seance [flags]
```

### Options

```
  -h, --help            help for seance
      --json            Output as JSON
  -p, --prompt string   One-shot prompt (with --talk)
  -n, --recent int      Number of recent sessions to show (default 20)
      --rig string      Filter by rig name
      --role string     Filter by role (crew, polecat, witness, etc.)
  -t, --talk string     Session ID to commune with
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

