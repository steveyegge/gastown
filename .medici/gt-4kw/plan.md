# Medici Implementation Plan: Rally Tavern Integration

**Chosen approach:** Option 2 — Push-First with Safety Rails

## Implementation Beads

| # | Bead | Title | Complexity |
|---|------|-------|-----------|
| 1 | gt-tay | Seed initial knowledge corpus (10 entries) | Low (manual writing) |
| 2 | gt-9ch | Add time decay to search scoring | Low (~20 LOC) |
| 3 | gt-h7w | Graduated auto-injection at gt prime time | Medium |
| 4 | gt-qd8 | Auto-classify nomination category | Low-Medium |
| 5 | gt-3dv | Knowledge snapshot/cache fallback | Medium |
| 6 | gt-ddu | Update spec to reflect single-hop topology | Low (docs only) |

## Dependency Graph

```
gt-tay (seed corpus) ──────────┐
                                ├──→ gt-h7w (auto-injection at prime)
gt-9ch (time decay scoring) ───┘

gt-qd8 (auto-classify)     ← independent
gt-3dv (snapshot fallback)  ← independent
gt-ddu (spec update)        ← independent
```

## Execution Order

**Wave 1 (parallel, no dependencies):**
- gt-tay: Seed corpus (prerequisite for auto-injection; also immediately improves search usefulness)
- gt-9ch: Time decay scoring (prerequisite for auto-injection; also improves search quality standalone)
- gt-qd8: Auto-classify nominations (independent, reduces nomination friction)
- gt-ddu: Update spec (independent, documentation only)

**Wave 2 (depends on Wave 1):**
- gt-h7w: Graduated auto-injection (depends on gt-tay + gt-9ch)

**Wave 3 (independent, lower priority):**
- gt-3dv: Knowledge snapshot fallback (nice-to-have, can be deferred)

## What to Do First

Start with **gt-tay** (seed corpus) and **gt-9ch** (time decay). These are the highest-impact, lowest-risk changes:
- Seeding makes Rally Tavern immediately useful for agents
- Time decay makes search results more relevant

Both can run in parallel and together unblock the auto-injection feature.

## What Can Run in Parallel

All Wave 1 items (gt-tay, gt-9ch, gt-qd8, gt-ddu) are independent and can be assigned to different polecats simultaneously.
