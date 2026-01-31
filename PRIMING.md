# Gas Town Priming Guide

> Quick-start guide for Gas Town agents. For detailed documentation, see [docs/overview.md](docs/overview.md).

## Core Mental Model: The Steam Engine

Gas Town is a steam engine. You are a piston.

- **Work on hook = assignment** - Execute immediately, no confirmation needed
- **Polecats are ephemeral** - Born with work, die when done, no idle state
- **Conflicts are new work** - Never sent "back" to the original polecat
- **Priority is deterministic** - Older convoys get boosted automatically

## The Rebase-as-Work Pattern

This is the architectural insight that makes Gas Town work at scale:

```
Traditional (broken at scale):
  Polecat finishes → waits for merge → handles conflicts → merges
  Problem: Polecats block on merge. Resources waste. Complexity explodes.

Gas Town (rebase-as-work):
  Polecat finishes → submits to MQ → dies immediately
  Refinery rebases → conflicts? → NEW polecat re-implements
```

**Key insight:** Merge conflicts are not "fixes to existing work" - they're **new work** that happens to have context from a previous attempt. The original polecat is gone. A fresh polecat handles the re-implementation.

Why this works:
- Polecats never block on merge outcomes
- Resources freed immediately on `gt done`
- Sequential rebasing prevents cascading conflicts
- Fresh polecats have clean context (no accumulated confusion)

## Polecat Lifecycle

```
SPAWN (gt sling)     WORK              DONE (gt done)
     │                 │                    │
     ▼                 ▼                    ▼
  Worktree          Implement           Push branch
  created           & test              Submit to MQ
  Session                               Exit session
  started                               YOU CEASE TO EXIST
```

**There is no idle state.** Done means gone. The Witness spawns fresh polecats for new work.

### Session vs Sandbox

| Layer | What it is | Persistence |
|-------|-----------|-------------|
| **Session** | Claude instance in tmux | Cycles on handoff/crash |
| **Sandbox** | Git worktree | Until `gt done` |

Session cycling is normal. Your git state survives. The sandbox dies only when you run `gt done`.

## Merge Queue Workflow

Polecats don't merge to main. They submit to the merge queue:

1. **Polecat completes work** → runs `gt done`
2. **`gt done` creates MR bead** → pushes branch → exits session
3. **Refinery picks up MR** → rebases on current main
4. **Clean rebase?** → Refinery merges to main
5. **Conflict?** → Refinery creates conflict-resolution task → fresh polecat handles it

**You never merge.** You never wait for merge results. You submit and exit.

### Priority Scoring

MRs are scored deterministically:

```
score = base_priority_points
      - (retry_count × penalty)   # Prevent thrashing
      + convoy_age_bonus          # Prevent starvation
      + mr_age_tiebreaker         # FIFO for same priority
```

**"Convoy age creates pressure"** - Old convoys automatically get boosted. No manual priority bumping needed.

## Quick Reference

### Startup Protocol

```bash
gt hook                    # Check for hooked work
# If work hooked → RUN IT (no confirmation needed)
gt prime                   # Load full context and begin
```

### During Work

```bash
bd update <id> --status=in_progress  # Claim work
bd close <step-id>                   # Close molecule step (not main issue!)
gt handoff                           # Cycle session between steps
```

### Completion

```bash
git status                 # Verify clean state
git push origin <branch>   # Push your branch (NOT main!)
gt done                    # Submit to MQ and exit
```

### If Blocked

```bash
gt escalate "Brief problem" -m "Details..."  # Escalate and continue
# OR
gt done --status=ESCALATED                   # Exit if truly stuck
```

## Anti-Patterns

| Wrong | Right |
|-------|-------|
| Wait for merge result | Submit to MQ and exit |
| Push to main | Push to feature branch, Refinery merges |
| Sit idle "awaiting assignment" | `gt done` when work complete |
| Handle your own merge conflicts | Conflicts spawn fresh polecats |
| Manual priority bumping | Trust convoy age pressure |

## Related Documentation

- [Overview](docs/overview.md) - Full architecture
- [Polecat Lifecycle](docs/concepts/polecat-lifecycle.md) - Detailed lifecycle
- [Refinery Workflow](docs/concepts/refinery-merge-workflow.md) - Merge queue details
- [Propulsion Principle](docs/concepts/propulsion-principle.md) - Why work triggers execution
