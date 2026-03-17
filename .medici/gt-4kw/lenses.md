# Medici Lenses: Rally Tavern ↔ Gas Town Integration

**Bead:** gt-4kw
**Lenses:** 6 (adapted from default set for a cross-rig integration review)

---

## 1. Domain Lens (`domain`)

**Question:** Does the knowledge model (YAML schema, tag taxonomy, search semantics) actually match how agents produce and consume knowledge in practice?

**Blind spot:** Assumes the knowledge schema was designed correctly upfront. May not notice that real agent knowledge doesn't fit neatly into practices/solutions/learned categories.

**What others are likely missing:** The gap between what agents *want* to look up (e.g., "how do I handle Dolt connection pooling in tests") and what the search API can express (exact tag match + substring). The search model may be too rigid for the messy reality of agent queries.

**Output:** `.medici/gt-4kw/domain.md`

---

## 2. Agent UX Lens (`agent-ux`)

**Question:** From a working polecat's perspective, does the knowledge integration actually get used? Is the friction low enough that agents benefit rather than just adding noise to their context?

**Blind spot:** Focuses on the happy path — agent finds relevant knowledge, applies it. May miss that most queries return nothing useful, wasting tokens and time.

**What others are likely missing:** The formula integration (mol-polecat-work, mol-idea-to-plan) makes knowledge lookup *advisory*. But advisory steps get skipped under pressure. If the step doesn't surface high-signal results immediately, agents will learn to ignore it — and the whole Rally Tavern loop becomes a dead feature.

**Output:** `.medici/gt-4kw/agent-ux.md`

---

## 3. Topology Lens (`topology`)

**Question:** Is the multi-hop nomination flow (polecat → rig Mayor → RT Mayor → franklin) the right topology for the current scale and trust model? What breaks as rigs multiply?

**Blind spot:** Thinks in terms of architecture diagrams and message flows. May miss that the actual bottleneck is human attention (or agent curator quality), not message routing.

**What others are likely missing:** Two-stage approval was designed for a future with many rigs and untrusted contributions. At current scale (~3 rigs, all trusted), it may add latency that kills the feedback loop. Nominations that take days to process won't motivate agents to nominate.

**Output:** `.medici/gt-4kw/topology.md`

---

## 4. Incentives Lens (`incentives`)

**Question:** Does the contribution loop actually incentivize agents to nominate good knowledge? What are the adoption dynamics?

**Blind spot:** Assumes that if you build the nomination command, agents will use it. May not examine *why* an agent would spend tokens nominating instead of moving to the next bead.

**What others are likely missing:** The nomination prompt in mol-polecat-work's self-clean step is the *only* nudge. If agents don't perceive value from Rally Tavern search results (because the corpus is thin early on), they won't nominate — creating a cold-start problem. The flywheel needs priming.

**Output:** `.medici/gt-4kw/incentives.md`

---

## 5. Constraints Lens (`constraints`)

**Question:** What are the hard operational constraints that could invalidate design assumptions? (CI environments, new dev setups, Dolt fragility, rally_tavern absence)

**Blind spot:** Focuses on what *can't* be done. May overweight constraints that are actually easy to work around while missing the one constraint that's truly binding.

**What others are likely missing:** The in-memory index model (load on every `gt rally search` invocation) means parsing 20-50 YAML files per search. At current corpus size this is fine, but the design has no caching layer — if the corpus grows 10x, every search pays full parse cost. Also: Dolt is the data plane for nominations. If Dolt is unhealthy (a known recurring issue), the contribution loop silently breaks.

**Output:** `.medici/gt-4kw/constraints.md`

---

## 6. Adversary Lens (`adversary`)

**Question:** How does this design fail? What are the second-order harms of bad knowledge, stale knowledge, or a broken contribution loop?

**Blind spot:** Sees threats everywhere. May flag theoretical risks that are unlikely at current scale while missing the mundane failure mode that's already happening.

**What others are likely missing:** Knowledge rot. There's a `gt rally report` command for flagging stale entries, but no automated staleness detection. If knowledge entries go stale (wrong patterns, outdated API advice), agents applying them will produce *worse* code than if Rally Tavern didn't exist. Bad knowledge is worse than no knowledge.

**Output:** `.medici/gt-4kw/adversary.md`

---

## Orthogonality Check

| Lens | Primary Axis | Likely Conclusion |
|------|-------------|-------------------|
| domain | Knowledge model fit | Schema may be too rigid for real agent queries |
| agent-ux | Adoption friction | Advisory steps get skipped; need high signal-to-noise |
| topology | Message flow architecture | Two-stage may be over-engineered for current scale |
| incentives | Adoption economics | Cold-start problem; flywheel needs priming |
| constraints | Operational limits | Index scaling + Dolt fragility are binding constraints |
| adversary | Failure modes | Knowledge rot is the existential threat |

No two lenses should produce the same answer. Each examines a different axis of the design.
