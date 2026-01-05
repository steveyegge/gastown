# Polecat Lifecycle

> Understanding the three-layer architecture of polecat workers

## Overview

Polecats have three distinct lifecycle layers that operate independently. Confusing
these layers leads to heresies like "idle polecats" and misunderstanding when
recycling occurs.

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
│                        gt done                              │
│  → Polecat signals completion to Witness                   │
│  → Session exits (no idle waiting)                         │
│  → Witness receives POLECAT_DONE event                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│               Witness: gt polecat nuke                      │
│  → Verify work landed (merged or in MQ)                    │
│  → Delete sandbox (remove worktree)                        │
│  → Kill tmux session                                       │
│  → Release slot back to pool                               │
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

### Idle Polecats

**Myth:** Polecats wait between tasks in an idle state.

**Reality:** Polecats don't exist without work. The lifecycle is:
1. Work assigned → polecat spawned
2. Work done → polecat nuked
3. There is no idle state

If you see a polecat without work, something is broken. Either:
- The hook was lost (bug)
- The session crashed before loading context
- Manual intervention corrupted state

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

### Sandboxes Without Work

**Anti-pattern:** A sandbox exists but no molecule is hooked.

This means:
- The polecat was spawned incorrectly
- The hook was lost during crash
- State corruption occurred

**Recovery:**
```bash
# From Witness:
gt polecat nuke Toast        # Clean slate
gt sling gt-abc gastown      # Respawn with work
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
- Recycle sandboxes between steps

The Witness DOES:
- Respawn crashed sessions
- Nudge stuck polecats
- Nuke completed polecats (after verification)
- Handle escalations

## Related Documentation

- [Understanding Gas Town](understanding-gas-town.md) - Role taxonomy and architecture
- [Polecat Wisp Architecture](polecat-wisp-architecture.md) - Molecule execution
- [Propulsion Principle](propulsion-principle.md) - Why work triggers immediate execution
