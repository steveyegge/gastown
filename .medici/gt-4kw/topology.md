# Medici Lens: Topology

**Issue:** gt-4kw — Rally Tavern ↔ Gas Town integration design review
**Bead:** gt-umg

## Framing

The nomination flow is a message bus pretending to be a pipeline: polecat → rig Mayor → rally_tavern Mayor (barkeep) → knowledge directory. Each hop is mediated by mail (Dolt-backed beads), with the barkeep as the terminal curator.

The topology question is: does this routing match the trust and authority model, and does it scale? Currently there are ~3 rigs, each with a Mayor. The two-stage approval (rig Mayor nominates, RT Mayor accepts) implies a trust boundary: rig Mayors filter for relevance, the barkeep filters for quality.

But in practice, rig Mayors don't currently have nomination-review logic. The `gt rally nominate` command sends directly to the barkeep. The "two-stage" design is aspirational — the implementation is single-hop (polecat → barkeep).

## What Other Lenses Will Likely Miss

**The rig Mayor hop is phantom infrastructure.** The Phase 1 plan specifies "two-stage — rig Mayor nominates, rally_tavern Mayor accepts" as a locked decision (Q5). But the implementation bypasses the rig Mayor entirely. `rally_nominate.go` sends mail directly to `rally_tavern/barkeep`.

This isn't a bug — it's practical. At 3 rigs, adding a Mayor-review step doubles latency for no clear quality improvement. But it means the "locked decision" is actually unlocked in practice. If someone later tries to implement the rig Mayor hop "because the spec says so," it will break the existing flow without adding value.

The spec and implementation have diverged. Either update the spec to match reality (single-hop), or implement the rig Mayor filtering. Don't leave the contradiction.

## Proposed Solution

**Formalize the single-hop topology for v1; design the two-hop as a v2 feature gated on rig count.**

1. Update `.specs/rally-tavern-phase1-plan.md` Q5 to: "Single-stage for v1 (direct to barkeep). Two-stage approval deferred until >5 rigs or untrusted contributors."
2. Add a `--via-mayor` flag to `gt rally nominate` that routes through the rig Mayor (not implemented, just reserved). Document when it should become the default.
3. Keep the current direct-to-barkeep path as the happy path.

This resolves the spec/implementation divergence without adding unnecessary routing.

## Failure Mode

**Barkeep bottleneck at scale.** With 3 rigs, the barkeep sees maybe 2-5 nominations per day. With 20 rigs, it could be 50+ nominations per patrol cycle. The barkeep is a single agent with a patrol formula — if nominations stack faster than it can process, the inbox grows, processing quality drops, and nominators stop getting feedback.

The rig Mayor hop was designed to prevent this by filtering before it reaches the barkeep. Removing it works now but creates a scaling wall. The experiment below tests where that wall is.

## Cheap Experiment

**Measure barkeep throughput.** Time how long the barkeep takes to process one nomination end-to-end (read mail → review → write YAML → commit → reply). Multiply by projected daily nominations at 5 rigs, 10 rigs, 20 rigs. If the barkeep can't process a day's nominations in one patrol cycle at 10 rigs, the two-hop topology needs to ship before hitting that rig count.
