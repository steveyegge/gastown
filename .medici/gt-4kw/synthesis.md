# Medici Synthesis: Rally Tavern ↔ Gas Town Integration Design Review

## Executive Summary

The Rally Tavern integration is well-implemented at the code level — the knowledge loader, search, nomination, and reporting all work. The design review surfaced no fundamental flaws in the implementation. Instead, it revealed **three structural gaps** that determine whether the system thrives or atrophies:

1. **The consumption model** (pull vs push) determines adoption
2. **The quality feedback loop** (verification + time decay) determines trust
3. **The cold-start problem** (seeding + incentives) determines whether the flywheel spins at all

These aren't bugs to fix — they're design decisions that need to be made explicitly before the pattern calcifies across rigs.

## What Each Lens Added

| Lens | Key Insight |
|------|-------------|
| **Domain** | The three-category taxonomy (practice/solution/learned) creates friction at nomination time. Classification should be automated or deferred, not required upfront. |
| **Agent UX** | Advisory formula steps get skipped. Knowledge injection must be push-based (at prime time) not pull-based (agent decides to search) for adoption. |
| **Topology** | The two-stage approval (rig Mayor → barkeep) in the spec isn't implemented. Current single-hop is correct for scale but the spec should be updated. |
| **Incentives** | Cold-start problem is real. Without manual corpus seeding, the flywheel won't spin. Nomination tracking in the capability ledger would close the feedback loop. |
| **Constraints** | Filesystem coupling (`$GT_ROOT/rally_tavern/`) is the binding constraint. A snapshot/cache fallback would decouple search from filesystem presence. |
| **Adversary** | Search scoring ignores verification status and age. Auto-injection without time decay amplifies wrong-knowledge blast radius. |

## Tensions That Matter

### Push vs Safety (Agent UX × Adversary)
Auto-injecting knowledge maximizes adoption but also maximizes the blast radius of wrong entries. **Resolution:** Graduated injection — verified entries auto-inject, unverified entries are pull-only. This was the primary collision finding.

### Simplicity vs Scale (Topology × Constraints)
The current single-hop, filesystem-coupled, in-memory design is simple and works at 3 rigs with 50 entries. But it has no caching, no distribution mechanism, and no routing for high nomination volume. **Resolution:** Don't over-engineer now, but identify the specific triggers that require each upgrade.

### Incentives vs Quality (Incentives × Domain)
Lowering nomination friction (auto-classify, reduce required fields) increases volume but may decrease quality. Higher-quality nominations require more agent effort, reducing contribution rate. **Resolution:** Auto-classify at nomination time, let barkeep re-classify at acceptance time.

## Candidate Approaches

### Option 1: Polish What Exists (minimal change)

**Description:** Fix the spec/implementation divergence (topology), add time decay to search scoring (adversary), seed 10 knowledge entries manually (incentives), and keep the current pull model.

**Why it could work:** The existing code is solid. Most of the Phase 1/2 plan is implemented. The system might bootstrap naturally once there's a critical mass of useful entries.

**Risks:** Pull model means adoption depends on agents voluntarily searching. If agents don't search, they don't find value, they don't nominate, and the corpus stays thin. This is a bet that the formula step instructions are enough.

**Who it serves best:** Teams that want minimal disruption and are willing to wait for organic adoption.

### Option 2: Push-First with Safety Rails (recommended)

**Description:** Implement graduated auto-injection at prime time (verified → auto-inject, unverified → pull-only), add time decay to search scoring, seed the corpus, add category auto-classification, and add a knowledge snapshot fallback.

**Why it could work:** Push maximizes the chance that agents encounter knowledge. Graduated injection limits blast radius. Time decay naturally deprioritizes stale entries. Corpus seeding bootstraps the flywheel. This addresses the three structural gaps identified across lenses.

**Risks:** More implementation work (~3-4 medium beads). Auto-injection adds tokens to every prime, even when irrelevant. The graduated model depends on agents verifying entries to keep them in the auto-inject tier.

**Who it serves best:** Teams that want Rally Tavern to be a living, actively-used system rather than an optional tool that might go unused.

### Option 3: Full Marketplace Redesign (ambitious)

**Description:** Everything in Option 2, plus: contribution tracking in the capability ledger, automated staleness detection in barkeep patrol, rig Mayor filtering for >5 rigs, remote knowledge endpoint for distributed setups, and fuzzy/semantic search.

**Why it could work:** This is the "Rally Tavern as a first-class platform" vision. It addresses every gap identified by every lens and scales to 20+ rigs.

**Risks:** Over-engineering for current scale. Many features solve problems that don't exist yet. 6+ months of work before seeing benefits. The system might not survive long enough to need these features.

**Who it serves best:** Teams building for a future with 10+ rigs and 100+ knowledge entries, and willing to invest heavily in the knowledge infrastructure.

## Recommended Approach

**Option 2: Push-First with Safety Rails.**

The **adversary** and **agent-ux** lenses together changed my thinking. Initially, the existing pull model seemed adequate — agents can search when they want. But agent-ux demonstrated convincingly that pull-based advisory steps get skipped, and adversary showed that naive push is dangerous. The collision between them produced the graduated injection model, which is the key insight of this review.

The **incentives** lens reinforced that corpus seeding isn't optional — it's prerequisite. Without visible, useful entries in the knowledge base, no amount of UX improvement drives adoption.

The **constraints** lens added the snapshot fallback, which is cheap to implement and eliminates the rally_tavern-must-exist assumption.

**Priority order for implementation:**
1. Seed 10 knowledge entries manually (unblocks everything else)
2. Add time decay to search scoring (~20 lines of Go)
3. Implement graduated auto-injection at prime time (medium complexity)
4. Add category auto-classification to `gt rally nominate` (small)
5. Add knowledge snapshot/cache fallback (medium)
6. Update spec to reflect single-hop topology (documentation only)

## Cheap Experiments

1. **Search hit-rate test** (from agent-ux): Run `gt rally search` against 5 recent bead descriptions. Measures current relevance.
2. **Corpus seeding** (from incentives): Write 10 entries, track search hits over 1 week.
3. **Barkeep throughput** (from topology): Time one end-to-end nomination processing cycle.
4. **Dolt-down nomination test** (from constraints): Stop Dolt, run `gt rally nominate`, observe failure mode.
5. **Staleness canary** (from adversary): Plant a deliberately wrong entry, measure time to detection.

## Open Questions

1. **Who seeds the initial corpus?** Manual human effort, a dedicated agent, or auto-generation from existing CLAUDE.md files?
2. **What's the token budget for auto-injected knowledge?** If 3 entries × 50 tokens = 150 tokens at prime, is that acceptable? What's the upper bound?
3. **Should verification be incentivized?** The graduated model depends on agents running `gt rally verify`. If they don't, all entries drift to pull-only. Should the barkeep proactively re-verify?
4. **When does the rig Mayor hop become necessary?** The topology lens suggests >5 rigs or untrusted contributors. Is there a more precise trigger?
