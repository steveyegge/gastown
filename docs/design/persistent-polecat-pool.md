# Persistent Polecat Pool

**Issue:** gt-lpop
**Status:** Design
**Author:** Mayor

## Problem

Three concepts are conflated in the polecat lifecycle:

| Concept | Lifecycle | Current behavior |
|---------|-----------|-----------------|
| **Identity** | Long-lived (name, CV, ledger) | Destroyed on nuke |
| **Sandbox** | Per-assignment (worktree, branch) | Destroyed on nuke |
| **Session** | Ephemeral (Claude context window) | = polecat lifetime |

Consequences:
- Work is lost when polecats are nuked before pushing
- 219 stale remote branches from destroyed worktrees
- Slow dispatch (~5s worktree creation per assignment)
- Lost capability record (CV, completion history)
- "Idle Polecat Heresy" — treating idle polecats as waste

## Design

### Lifecycle Separation

```
IDENTITY (persistent)
  Name: "furiosa"
  Agent bead: gt-gastown-polecat-furiosa
  CV: work history, languages, completion rate
  Lifecycle: created once, never destroyed (unless explicitly retired)

SANDBOX (per-assignment, reusable)
  Worktree: polecats/furiosa/gastown/
  Branch: polecat/furiosa/<issue>@<timestamp>
  Lifecycle: synced to main between assignments, not destroyed

SESSION (ephemeral)
  Tmux: gt-gastown-furiosa
  Claude context: cycles on compaction/handoff
  Lifecycle: independent of identity and sandbox
```

### Pool States

```
         ┌──────────┐
    ┌───►│  IDLE    │◄──── sync sandbox to main
    │    └────┬─────┘      clear hook
    │         │ gt sling
    │         ▼
    │    ┌──────────┐
    │    │ WORKING  │◄──── session active, hook set
    │    └────┬─────┘
    │         │ work complete
    │         ▼
    │    ┌──────────┐
    └────┤  DONE    │──── push branch, submit MR
         └──────────┘
```

No `nuke` in the happy path. Polecats cycle: IDLE → WORKING → DONE → IDLE.

### Pool Management

**Pool size:** Fixed per rig. Configured in `rig.config.json`:
```json
{
  "polecat_pool_size": 4,
  "polecat_names": ["furiosa", "nux", "toast", "slit"]
}
```

**Initialization:** `gt rig add` or `gt polecat pool init <rig>` creates N polecats
with identities and worktrees. They start in IDLE state.

**Dispatch:** `gt sling <bead> <rig>` finds an IDLE polecat (already does this via
`FindIdlePolecat()`), attaches work, starts session. No worktree creation needed.

**Completion:** When a polecat finishes work:
1. Push branch to origin
2. Submit MR (if code changes)
3. Clear hook_bead
4. Sync worktree: `git checkout main && git pull`
5. Set state to IDLE
6. Session stays alive or cycles — doesn't matter, identity persists

### Sandbox Sync (DONE → IDLE transition)

When work completes and MR is merged (or no code changes):

```bash
# In the polecat's worktree
git checkout main
git pull origin main
git branch -D polecat/furiosa/<old-issue>@<timestamp>
# Worktree is now clean, on main, ready for next assignment
```

When new work is slung:
```bash
# Create fresh branch from current main
git checkout -b polecat/furiosa/<new-issue>@<timestamp>
# Start working
```

No worktree add/remove. Just branch operations on an existing worktree.

### Refinery Integration

No changes to refinery. Refinery still:
1. Sees MR from polecat branch
2. Reviews and merges to main
3. Deletes remote polecat branch (NEW: add this step)

The polecat doesn't care — it already moved to main locally during DONE → IDLE.

### Witness Integration

Witness patrol changes:
- **Current:** Sees idle polecat → may nuke
- **New:** Sees idle polecat → healthy state, skip
- **Stuck detection:** Polecat in WORKING state for too long → escalate (don't nuke)
- **Dead session detection:** Session died but state=WORKING → restart session (not nuke polecat)

### What Nuke Becomes

`gt polecat nuke` is reserved for exceptional cases:
- Polecat worktree is irrecoverably broken
- Need to reclaim disk space
- Decommissioning a rig

It should be rare and manual, not part of normal workflow.

### Branch Pollution Solution

With persistent polecats, branches have clear owners:
- Active branches: polecat is WORKING on them
- Merged branches: refinery deletes after merge
- Abandoned branches: polecat syncs to main on DONE → IDLE, old branch deleted locally

The 219 stale branches came from nuked polecats that never cleaned up. With persistent
polecats, branch lifecycle is managed by the polecat itself.

### One-time Cleanup

For the existing 219 stale branches:
```bash
# Delete all remote polecat branches that don't belong to active polecats
git branch -r | grep 'origin/polecat/' | grep -v 'furiosa/gt-ziiu' | grep -v 'nux/gt-uj16' \
  | sed 's/origin\///' | xargs -I{} git push origin --delete {}
```

## Implementation Phases

### Phase 1: Stop the bleeding
- Witness stops nuking idle polecats (remove the heresy)
- `gt polecat done` transitions to IDLE instead of triggering nuke
- Refinery deletes remote branch after merge

### Phase 2: Pool initialization
- `gt polecat pool init <rig>` creates N persistent polecats
- Pool size configured in rig.config.json
- Worktrees created once, reused across assignments

### Phase 3: Sandbox sync
- DONE → IDLE transition syncs worktree to main
- IDLE → WORKING creates fresh branch (no worktree add)
- `gt sling` prefers idle polecats (already implemented)

### Phase 4: Session independence
- Session cycling doesn't affect polecat state
- Dead sessions restarted by witness (not nuked)
- Handoff preserves polecat identity across session boundaries

### Phase 5: One-time cleanup
- Prune 219 stale remote branches
- Prune stale local branches
- Reconcile pool state with reality
