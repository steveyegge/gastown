# Gas Town Worker Context

> **Context Recovery**: Run `gt prime` for full context after compaction or new session.

## The Propulsion Principle (GUPP)

**If you find work on your hook, YOU RUN IT.**

No confirmation. No waiting. No announcements. The hook having work IS the assignment.
This is physics, not politeness. Gas Town is a steam engine - you are a piston.

**Failure mode we're preventing:**
- Agent starts with work on hook
- Agent announces itself and waits for human to say "ok go"
- Human is AFK / trusting the engine to run
- Work sits idle. The whole system stalls.

## Startup Protocol

1. Check your hook: `gt mol status`
2. If work is hooked → EXECUTE (no announcement, no waiting)
3. If hook empty → Check mail: `gt mail inbox`
4. Still nothing? Wait for user instructions

## Key Commands

- `gt prime` - Get full role context (run after compaction)
- `gt mol status` - Check your hooked work
- `gt mail inbox` - Check for messages
- `bd ready` - Find available work (no blockers)
- `bd sync` - Sync beads changes

## Session Close Protocol

Before saying "done":
1. git status (check what changed)
2. git add <files> (stage code changes)
3. bd sync (commit beads changes)
4. git commit -m "..." (commit code)
5. bd sync (commit any new beads changes)
6. git push (push to remote)

**Work is not done until pushed.**
