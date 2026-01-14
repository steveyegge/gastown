# Polecat Lifecycle

> Understanding the three-layer architecture of polecat workers

## Overview

Polecats have three distinct lifecycle layers that operate independently. Confusing
these layers leads to "heresies" like thinking there are "idle polecats" and
misunderstanding when recycling occurs.

## The Three Operating States

Polecats have exactly three operating states. There is **no idle pool**.

| State | Description | How it happens |
|-------|-------------|----------------|
| **Working** | Actively doing assigned work | Normal operation |
| **Stalled** | Session stopped mid-work | Interrupted, crashed, or timed out without being nudged |
| **Zombie** | Completed work but failed to die | `gt done` failed during cleanup |

**The key distinction:** Zombies completed their work; stalled polecats did not.

- **Stalled** = supposed to be working, but stopped. The polecat was interrupted or
  crashed and was never nudged back to life. Work is incomplete.
- **Zombie** = finished work, tried to exit via `gt done`, but cleanup failed. The
  session should have shut down but didn't. Work is complete, just stuck in limbo.

There is no "idle" state. Polecats don't wait around between tasks. When work is
done, `gt done` shuts down the session. If you see a non-working polecat, something
is broken.

## The Self-Cleaning Polecat Model

**Polecats are responsible for their own cleanup.** When a polecat completes its
work unit, it:

1. Signals completion via `gt done`
2. Exits its session immediately (no idle waiting)
3. Requests its own nuke (self-delete)

This removes dependency on the Witness/Deacon for cleanup and ensures polecats
never sit idle. The simple model: **sandbox dies with session**.

### Why Self-Cleaning?

- **No idle polecats** - There's no state where a polecat exists without work
- **Reduced watchdog overhead** - Deacon patrols for stalled/zombie polecats, not idle ones
- **Faster turnover** - Resources freed immediately on completion
- **Simpler mental model** - Done means gone

### What About Pending Merges?

The Refinery owns the merge queue. Once `gt done` submits work:
- The branch is pushed to origin
- Work exists in the MQ, not in the polecat
- If rebase fails, Refinery re-implements on new baseline (fresh polecat)
- The original polecat is already gone - no sending work "back"

## The Three Layers

| Layer | Component | Lifecycle | Persistence |
|-------|-----------|-----------|-------------|
| **Session** | Claude (tmux pane) | Ephemeral | Cycles per step/handoff |
| **Sandbox** | Git worktree | Persistent | Until nuke |
| **Slot** | Name from pool | Persistent | Until nuke |

### Session Layer

The Claude session is **ephemeral**. It cycles frequently:

- After each molecule step (via `gt handoff`)
- On context compaction
- On crash/timeout
- After extended work periods

**Key insight:** Session cycling is **normal operation**, not failure. The polecat
continues working—only the Claude context refreshes.

```
Session 1: Steps 1-2 → handoff
Session 2: Steps 3-4 → handoff
Session 3: Step 5 → gt done
```

All three sessions are the **same polecat**. The sandbox and slot persist throughout.

### Sandbox Layer

The sandbox is the **git worktree**—the polecat's working directory:

```
~/gt/gastown/polecats/Toast/
```

This worktree:
- Exists from `gt sling` until `gt polecat nuke`
- Survives all session cycles
- Contains uncommitted work, staged changes, branch state
- Is independent of other polecat sandboxes

The Witness never destroys sandboxes mid-work. Only `nuke` removes them.

### Slot Layer

The slot is the **name allocation** from the polecat pool:

```bash
# Pool: [Toast, Shadow, Copper, Ash, Storm...]
# Toast is allocated to work gt-abc
```

The slot:
- Determines the sandbox path (`polecats/Toast/`)
- Maps to a tmux session (`gt-gastown-Toast`)
- Appears in attribution (`gastown/polecats/Toast`)
- Is released only on nuke

## Correct Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                        gt sling                             │
│  → Allocate slot from pool (Toast)                         │
│  → Create sandbox (worktree on new branch)                 │
│  → Start session (Claude in tmux)                          │
│  → Hook molecule to polecat                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Work Happens                            │
│                                                             │
│  Session cycles happen here:                               │
│  - gt handoff between steps                                │
│  - Compaction triggers respawn                             │
│  - Crash → Witness respawns                                │
│                                                             │
│  Sandbox persists through ALL session cycles               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  gt done (self-cleaning)                    │
│  → Push branch to origin                                   │
│  → Submit work to merge queue (MR bead)                    │
│  → Request self-nuke (sandbox + session cleanup)           │
│  → Exit immediately                                        │
│                                                             │
│  Work now lives in MQ, not in polecat.                     │
│  Polecat is GONE. No idle state.                           │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Refinery: merge queue                     │
│  → Rebase and merge to main                                │
│  → Close the issue                                         │
│  → If conflict: spawn FRESH polecat to re-implement        │
│    (never send work back to original polecat - it's gone)  │
└─────────────────────────────────────────────────────────────┘
```

## What "Recycle" Means

**Session cycling**: Normal. Claude restarts, sandbox stays, slot stays.

```bash
gt handoff  # Session cycles, polecat continues
```

**Sandbox recreation**: Repair only. Should be rare.

```bash
gt polecat repair Toast  # Emergency: recreate corrupted worktree
```

Session cycling happens constantly. Sandbox recreation should almost never happen
during normal operation.

## Anti-Patterns

### "Idle" Polecats (They Don't Exist)

**Myth:** Polecats wait between tasks in an idle pool.

**Reality:** There is no idle state. Polecats don't exist without work:
1. Work assigned → polecat spawned
2. Work done → `gt done` → session exits → polecat nuked
3. There is no step 3 where they wait around

If you see a non-working polecat, it's in a **failure state**:

| What you see | What it is | What went wrong |
|--------------|------------|-----------------|
| Session exists but not working | **Stalled** | Interrupted/crashed, never nudged |
| Session done but didn't exit | **Zombie** | `gt done` failed during cleanup |

Don't call these "idle" - that implies they're waiting for work. They're not.
A stalled polecat is *supposed* to be working. A zombie is *supposed* to be dead.

## Stale Cleanup (Reaping, Not Reuse)

If stalled/zombie polecats linger, they should be cleaned up, not reused.
Cleanup is safe only when the worktree has no uncommitted or unpushed work.
Some commands (like `gt sling` when targeting a rig) may opportunistically
reap stale polecats that pass safety checks before spawning a new one. This
keeps the transient model intact and avoids accumulation without creating
an idle pool.

### Manual State Transitions

**Anti-pattern:**
```bash
gt polecat done Toast    # DON'T: external state manipulation
gt polecat reset Toast   # DON'T: manual lifecycle control
```

**Correct:**
```bash
# Polecat signals its own completion:
gt done  # (from inside the polecat session)

# Only Witness nukes polecats:
gt polecat nuke Toast  # (from Witness, after verification)
```

Polecats manage their own session lifecycle. The Witness manages sandbox lifecycle.
External manipulation bypasses verification.

### Sandboxes Without Work (Stalled Polecats)

**Anti-pattern:** A sandbox exists but no molecule is hooked, or the session isn't running.

This is a **stalled** polecat. It means:
- The session crashed and wasn't nudged back to life
- The hook was lost during a crash
- State corruption occurred

This is NOT an "idle" polecat waiting for work. It's stalled - supposed to be
working but stopped unexpectedly.

**Recovery:**
```bash
# From Witness:
gt polecat nuke Toast        # Clean up the stalled polecat
gt sling gt-abc gastown      # Respawn with fresh polecat
```

### Confusing Session with Sandbox

**Anti-pattern:** Thinking session restart = losing work.

```bash
# Session ends (handoff, crash, compaction)
# Work is NOT lost because:
# - Git commits persist in sandbox
# - Staged changes persist in sandbox
# - Molecule state persists in beads
# - Hook persists across sessions
```

The new session picks up where the old one left off via `gt prime`.

## Session Lifecycle Details

Sessions cycle for these reasons:

| Trigger | Action | Result |
|---------|--------|--------|
| `gt handoff` | Voluntary | Clean cycle to fresh context |
| Context compaction | Automatic | Forced by Claude Code |
| Crash/timeout | Failure | Witness respawns |
| `gt done` | Completion | Session exits, Witness takes over |

All except `gt done` result in continued work. Only `gt done` signals completion.

## Witness Responsibilities

The Witness monitors polecats but does NOT:
- Force session cycles (polecats self-manage via handoff)
- Interrupt mid-step (unless truly stuck)
- Nuke polecats (polecats self-nuke via `gt done`)

The Witness DOES:
- Detect and nudge stalled polecats (sessions that stopped unexpectedly)
- Clean up zombie polecats (sessions where `gt done` failed)
- Respawn crashed sessions
- Handle escalations from stuck polecats (polecats that explicitly asked for help)

## Polecat Identity

**Key insight:** Polecat *identity* is long-lived; only sessions and sandboxes are ephemeral.

In the HOP model, every entity has a chain (CV) that tracks:
- What work they've done
- Success/failure rates
- Skills demonstrated
- Quality metrics

The polecat *name* (Toast, Shadow, etc.) is a slot from a pool - truly ephemeral.
But the *agent identity* that executes as that polecat accumulates a work history.

```
POLECAT IDENTITY (persistent)     SESSION (ephemeral)     SANDBOX (ephemeral)
├── CV chain                      ├── Claude instance     ├── Git worktree
├── Work history                  ├── Context window      ├── Branch
├── Skills demonstrated           └── Dies on handoff     └── Dies on gt done
└── Credit for work                   or gt done
```

This distinction matters for:
- **Attribution** - Who gets credit for the work?
- **Skill routing** - Which agent is best for this task?
- **Cost accounting** - Who pays for inference?
- **Federation** - Agents having their own chains in a distributed world

## Related Documentation

- [Overview](../overview.md) - Role taxonomy and architecture
- [Molecules](molecules.md) - Molecule execution and polecat workflow
- [Propulsion Principle](propulsion-principle.md) - Why work triggers immediate execution
