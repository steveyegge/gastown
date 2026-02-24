# Polecat Lifecycle

> Understanding the three-layer architecture of polecat workers

## Overview

Polecats have three distinct lifecycle layers that operate independently. The
key design principle: **polecats are persistent**. They survive work completion
and can be reused across assignments.

## The Four Operating States

Polecats have four operating states:

| State | Description | How it happens |
|-------|-------------|----------------|
| **Working** | Actively doing assigned work | Normal operation after `gt sling` |
| **Idle** | Work completed, sandbox preserved for reuse | After `gt done` completes successfully |
| **Stalled** | Session stopped mid-work | Interrupted, crashed, or timed out without being nudged |
| **Zombie** | Completed work but failed to exit | `gt done` failed during cleanup |

**Key distinctions:**

- **Working** = actively executing. Session alive, hook set, doing work.
- **Idle** = work done, session killed, sandbox preserved. Ready for next `gt sling`.
- **Stalled** = supposed to be working, but stopped. Needs Witness intervention.
- **Zombie** = finished work, tried to exit, but cleanup failed. Stuck in limbo.

## The Persistent Polecat Model (gt-4ac)

**Polecats persist after completing work.** When a polecat finishes its assignment:

1. Signals completion via `gt done`
2. Pushes branch, submits MR to merge queue
3. Clears its hook (work is done)
4. Sets agent state to "idle"
5. Kills its own session
6. **Sandbox (worktree) is preserved for reuse**

The next `gt sling` reuses idle polecats before allocating new ones, avoiding
the overhead of creating fresh worktrees.

### Why Persistent?

- **Faster turnaround** — Reusing an existing worktree is faster than creating one
- **Preserved identity** — The polecat's agent bead, CV chain, and work history persist
- **Simpler lifecycle** — No nuke/respawn cycle between assignments
- **Done means idle** — Session dies, sandbox lives, polecat awaits next assignment

### What About Pending Merges?

The Refinery owns the merge queue. Once `gt done` submits work:
- The branch is pushed to origin
- Work exists in the MQ, not in the polecat
- If rebase fails, Refinery creates a conflict-resolution task
- The idle polecat can be reused for the conflict resolution work

## The Three Layers

| Layer | Component | Lifecycle | Persistence |
|-------|-----------|-----------|-------------|
| **Identity** | Agent bead, CV chain, work history | Permanent | Never dies |
| **Sandbox** | Git worktree, branch | Persistent across assignments | Created on first sling, reused thereafter |
| **Session** | Claude (tmux pane), context window | Ephemeral per step | Cycles per step/handoff |

### Identity Layer

The polecat's **identity is permanent**. It includes:

- Agent bead (created once, never deleted)
- CV chain (work history accumulates across all assignments)
- Mailbox and attribution record

Identity survives all session cycles and sandbox resets. In the HOP model, this IS
the polecat — everything else is infrastructure that comes and goes. See
[Polecat Identity](#polecat-identity) below for details.

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

All three sessions are the **same polecat**. The sandbox persists throughout.

### Sandbox Layer

The sandbox is the **git worktree**—the polecat's working directory:

```
~/gt/gastown/polecats/Toast/
```

This worktree:
- Exists from first `gt sling` and persists across assignments
- Survives all session cycles
- Is repaired (reset to fresh branch from main) when reused by `gt sling`
- Contains uncommitted work, staged changes, branch state during active work

The Witness never destroys sandboxes. Only explicit `gt polecat nuke` removes them.

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
- Persists until explicit nuke

## Correct Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                        gt sling                             │
│  → Find idle polecat OR allocate slot from pool (Toast)    │
│  → Create/repair sandbox (worktree on new branch)          │
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
│                  gt done (persistent model)                  │
│  → Push branch to origin                                   │
│  → Submit work to merge queue (MR bead)                    │
│  → Set agent state to "idle"                               │
│  → Kill session                                            │
│                                                             │
│  Work now lives in MQ. Polecat is IDLE, not gone.          │
│  Sandbox preserved for reuse by next gt sling.             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Refinery: merge queue                     │
│  → Rebase and merge to target branch                       │
│    (main or integration branch — see below)                │
│  → Close the issue                                         │
│  → If conflict: create task for available polecat          │
│                                                             │
│  Integration branch path:                                  │
│  → MRs from epic children merge to integration/<epic>      │
│  → When all children closed: land to main as one commit    │
└─────────────────────────────────────────────────────────────┘
```

## What "Recycle" Means

**Session cycling**: Normal. Claude restarts, sandbox stays, slot stays.

```bash
gt handoff  # Session cycles, polecat continues
```

**Sandbox repair**: On reuse. `gt sling` resets the worktree to a fresh branch.

```bash
gt sling gt-xyz gastown  # Reuses idle Toast, repairs worktree
```

Session cycling happens constantly. Sandbox repair happens between assignments.

## Anti-Patterns

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

# Only explicit nuke destroys polecats:
gt polecat nuke Toast  # (destroys sandbox, identity persists)
```

Polecats manage their own session lifecycle. External manipulation bypasses verification.

### Sandboxes Without Work (Idle vs Stalled)

An idle polecat has no hook and no session — this is **normal**. It completed
its work and is waiting for the next `gt sling`.

A **stalled** polecat has a hook but no session — this is a **failure**:
- The session crashed and wasn't nudged back to life
- The hook was lost during a crash
- State corruption occurred

**Recovery for stalled:**
```bash
# Witness respawns the session in the existing sandbox
# Or, if unrecoverable:
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
| `gt done` | Completion | Session exits, polecat goes idle |

All except `gt done` result in continued work. Only `gt done` signals completion
and transitions the polecat to idle.

## Witness Responsibilities

The Witness monitors polecats but does NOT:
- Force session cycles (polecats self-manage via handoff)
- Interrupt mid-step (unless truly stuck)
- Nuke polecats after completion (persistent model)

The Witness DOES:
- Detect and nudge stalled polecats (sessions that stopped unexpectedly)
- Clean up zombie polecats (sessions where `gt done` failed)
- Respawn crashed sessions
- Handle escalations from stuck polecats (polecats that explicitly asked for help)

## Polecat Identity

**Key insight:** Polecat *identity* is permanent; sessions are ephemeral, sandboxes are persistent.

In the HOP model, every entity has a chain (CV) that tracks:
- What work they've done
- Success/failure rates
- Skills demonstrated
- Quality metrics

The polecat *name* (Toast, Shadow, etc.) is a slot from a pool — persistent until
explicit nuke. The *agent identity* that executes as that polecat accumulates a
work history across all assignments.

```
POLECAT IDENTITY (permanent)      SESSION (ephemeral)     SANDBOX (persistent)
├── CV chain                      ├── Claude instance     ├── Git worktree
├── Work history                  ├── Context window      ├── Branch
├── Skills demonstrated           └── Dies on handoff     └── Repaired on reuse
└── Credit for work                   or gt done              by gt sling
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
