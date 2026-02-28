# Dog Execution Model: Imperative vs Formula Dispatch

## Status: Active Design Doc
Created: 2026-02-27

## Problem Statement

Gas Town dogs (daemon patrol routines) use two execution models:

1. **Imperative Go** (ticker fires → Go code runs): Doctor, Reaper, JSONL backup, Dolt backup
2. **Formula-only** (ticker fires → molecule poured → ... nothing): Compactor (was stub), Janitor

The formula-only dogs were broken because no agent interprets their molecules from
ticker context. The molecule system requires an idle dog to execute the formula, but
the ticker fires regardless of dog availability.

After the Beads Flows work, the Compactor has been upgraded to imperative Go. This
document captures the target execution model going forward.

## Current State (Post Beads Flows)

| Dog | Model | Works? | Notes |
|-----|-------|--------|-------|
| Doctor | Imperative Go (466 lines) | Yes | 7 health checks, GC, zombie kill |
| Reaper | Imperative Go (658 lines) | Yes | Close, purge, auto-close, mail purge |
| JSONL Backup | Imperative Go (619 lines) | Yes | Export, scrub, filter, spike detect, push |
| Dolt Backup | Imperative Go | Yes | Filesystem backup sync |
| Compactor | Imperative Go (new) | Yes | Flatten + GC when commits > threshold |
| Janitor | Formula-only (stub) | No | Pours mol-dog-janitor, nothing executes it |

## Target Model

### Keep imperative Go for: reliability-critical dogs

Dogs that MUST run on schedule, unattended, with no agent dependency:

- **Doctor**: Health checks are the foundation. Must run even if all agents are dead.
- **Reaper**: Data hygiene can't depend on agent availability.
- **Compactor**: Compaction must run deterministically on its 24h schedule.
- **JSONL Backup**: Backup integrity can't be left to agent scheduling.
- **Dolt Backup**: Same as JSONL.

**Principle**: If the dog's failure would cause a Clown Show, it must be imperative Go.

### Migrate to plugin dispatch for: enhancement/opportunistic dogs

Dogs whose failure is merely inconvenient, not catastrophic:

- **Janitor**: Test server orphan cleanup. Nice to have, not critical.
- Future: cosmetic cleanup, metrics collection, log rotation.

### Plugin dispatch model

For plugin-dispatched dogs:

1. Remove dedicated ticker from daemon `Run()` loop
2. Create `plugins/<dog>/plugin.md` with cooldown gate
3. `handleDogs()` dispatches to idle dog when cooldown expires
4. Dog agent interprets the plugin formula and executes

**Key constraint**: The `handleDogs()` dispatch path already exists and works.
The issue is that ticker-based dogs bypass it. Plugin dogs use it correctly.

## Migration Path

### Phase 1: Janitor (prototype)
- Move from ticker → plugin dispatch
- Create `plugins/janitor/plugin.md`
- Verify cleanup still happens when dog is idle

### Phase 2: Evaluate results
- Did the janitor run often enough?
- Did the formula+agent model add latency?
- Any reliability concerns?

### Phase 3: Future dogs default to plugin
- New dogs should start as plugins unless reliability-critical
- Existing imperative dogs stay as Go (working, tested, reliable)

## Decision: Do NOT migrate working imperative dogs

The Doctor, Reaper, Compactor, and backup dogs work reliably as imperative Go.
Migrating them to formula+agent would:

1. Add a dependency on agent availability
2. Introduce latency (agent startup, formula interpretation)
3. Risk regression on critical paths
4. Gain nothing — they already work

**The only dogs that should use formula dispatch are ones where agent intelligence
adds value** (e.g., the janitor deciding which orphans are safe to remove based on
context) or where the dog's task is inherently non-critical.
