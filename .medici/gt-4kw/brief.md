# Medici Brief: Rally Tavern ↔ Gas Town Integration Design Review

**Bead:** gt-4kw
**Date:** 2026-03-16
**Source:** `.specs/rally-tavern-phase1-plan.md`, existing implementation in `internal/rally/` and `internal/cmd/rally*.go`

## Core Problem

Rally Tavern is a shared knowledge repository (`$GT_ROOT/rally_tavern/`) that Gas Town agents need to query during planning and implementation. The integration design establishes how Gas Town's `gt` CLI discovers, searches, and contributes to this knowledge base — and how this feeds into formula-driven workflows (mol-idea-to-plan, mol-polecat-work).

This is a cross-cutting design affecting multiple rigs and setting the pattern for all future knowledge-sharing across Gas Town instances.

## Who Is Affected

- **Polecats (all rigs)**: consume knowledge during `implement` steps via `gt rally search/lookup`
- **Mayors**: route nomination approvals (two-stage: rig Mayor → RT Mayor)
- **Franklin (rally_tavern curator)**: processes nominations from all rigs
- **Barkeep**: handles nomination lifecycle in rig context
- **Future rigs**: any new rig that wants to share/consume knowledge follows this pattern

## Why This Is Hard / Cross-Cutting

1. **Multi-rig coordination**: Knowledge flows across rig boundaries (polecat → rig Mayor → rally_tavern Mayor → franklin). Each hop crosses a trust/authority boundary.
2. **Graceful degradation**: rally_tavern may not exist. Every touchpoint must handle absence without crashing or blocking agents.
3. **Formula injection**: Knowledge must be woven into formula steps without making them rally_tavern-dependent — advisory, not blocking.
4. **Search quality vs simplicity**: Tag-exact + substring matching is simple but may not surface relevant knowledge. The gap between "simple works" and "needs embeddings" is hard to judge without real usage data.
5. **Contribution loop**: Nominations must survive agent session death, traverse rig boundaries, and get human-quality review from an agent curator.

## Known Constraints

- Pure Go reimplementation — no dependency on rally_tavern bash scripts
- Knowledge-only in v1 (artifacts stay in existing bash scripts)
- In-memory index, no persistence (corpus is small: ~20-50 files)
- Two-stage approval routing is architecturally locked
- `$GT_ROOT/rally_tavern/` location is locked

## What Success Looks Like

- Agents naturally discover relevant knowledge during planning and implementation without manual intervention
- Knowledge contributions flow from polecats → rally_tavern without human bottlenecks
- The pattern is replicable for future cross-rig knowledge sharing
- rally_tavern absence is invisible — no errors, no degraded agent behavior

## Current Implementation State

Most of the Phase 1 and Phase 2 plan is **already implemented**:

| Feature | Status |
|---------|--------|
| Knowledge Index Loader (`internal/rally/knowledge.go`) | ✅ Done |
| Tavern Profile Reader (`internal/rally/profile.go`) | ✅ Done |
| `gt rally search` | ✅ Done |
| `gt rally lookup` | ✅ Done |
| `gt rally nominate` | ✅ Done |
| `gt rally report` | ✅ Done |
| Formula integration (mol-idea-to-plan) | Needs verification |
| Formula integration (mol-polecat-work) | Needs verification |
| Nomination lifecycle (barkeep plugin) | ✅ Done |

## Is a Medici Round Warranted?

**Yes, but scoped.** The implementation is largely complete, so a full from-scratch design review would be wasteful. However, a focused Medici review is warranted because:

1. **Cross-cutting pattern review**: This establishes THE pattern for inter-rig knowledge sharing. Getting it wrong means every future integration inherits the mistakes.
2. **Several open design tensions** deserve multi-lens examination:
   - CLI design: `gt rally search` is bolt-on; should it be more deeply integrated (e.g., auto-injected into agent context)?
   - Search quality: exact-match + substring is a v1 bet — is there evidence it's sufficient or do we need to plan for fuzzy/semantic search sooner?
   - Nomination routing: two-stage approval may be over-engineered for current scale (~3 rigs). Does it add latency that kills the feedback loop?
   - Location model: hardcoded `$GT_ROOT/rally_tavern/` — what happens when Gas Town runs in environments without rally_tavern (CI, new dev setups)?
3. **Retrospective value**: Code exists but hasn't been reviewed through orthogonal lenses. A Medici round can surface blind spots in the existing implementation before the pattern calcifies.

**Recommendation**: Proceed with Medici, but focus lenses on the pattern-setting aspects (inter-rig knowledge flow, search UX, contribution loop health) rather than individual function implementations.
