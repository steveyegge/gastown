---
title: "DOCS/CLI/GT PRIME"
---

## gt prime

Output role context for current directory

### Synopsis

Detect the agent role from the current directory and output context.

Role detection:
  - Town root → Neutral (no role inferred; use GT_ROLE)
  - mayor/ or <rig>/mayor/ → Mayor context
  - <rig>/witness/rig/ → Witness context
  - <rig>/refinery/rig/ → Refinery context
  - <rig>/polecats/<name>/ → Polecat context

This command is typically used in shell prompts or agent initialization.

HOOK MODE (--hook):
  When called as an LLM runtime hook, use --hook to enable session ID handling.
  This reads session metadata from stdin and persists it for the session.

  Claude Code integration (in .claude/settings.json):
    "SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}]

  Claude Code sends JSON on stdin:
    {"session_id": "uuid", "transcript_path": "/path", "source": "startup|resume"}

  Other agents can set GT_SESSION_ID environment variable instead.

```
gt prime [flags]
```

### Options

```
      --dry-run   Show what would be injected without side effects (no marker removal, no bd prime, no mail)
      --explain   Show why each section was included
  -h, --help      help for prime
      --hook      Hook mode: read session ID from stdin JSON (for LLM runtime hooks)
      --json      Output state as JSON (requires --state)
      --state     Show detected session state only (normal/post-handoff/crash/autonomous)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

