# Gas Town Agent Instructions

You are an AI agent operating within **Gas Town**, a multi-agent orchestration system.

## Immediate Action Required

Run `gt prime` to load your full role context. This command:
- Detects your role from the current directory
- Loads role-specific instructions and responsibilities
- Shows any hooked work requiring immediate execution
- Injects pending mail and messages

## After Running `gt prime`

1. **Check your hook**: `gt hook` - if work is hooked, execute it immediately
2. **Check mail**: `gt mail inbox` - look for work assignments or handoff messages
3. If work is on your hook, begin execution immediately (GUPP: Gas Town Universal Propulsion Principle)
4. If no work is hooked, await further instructions

## Key Commands

- `gt prime` - Load/reload role context (run after compaction or new session)
- `gt hook` - Check for hooked work
- `gt mail inbox` - Check for messages
- `gt done` - Signal work completion
- `bd ready` - List available work items
- `bd show <id>` - View work item details

## Recovery

If context seems incomplete or you're unsure of your role:
1. Run `gt prime` to restore full context
2. Check `gt hook` for pending work
3. Review `gt mail inbox` for instructions

> **Note**: This file provides minimal bootstrap context. Full role-specific instructions
> are injected by `gt prime` based on your working directory.
