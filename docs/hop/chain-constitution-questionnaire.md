# Chain Constitution Parameter Questionnaire

> **Version:** 0.1 (draft)
> **Related:** [HOP Protocol Spec](protocol-spec.md), w-hop-002

## Purpose

Before launching a new federation chain, founders must define its
**constitution** -- the governance rules that determine how participants
interact, how trust is earned, how disputes are resolved, and how the
chain evolves over time.

This questionnaire helps chain founders think through the key decisions.
Each question includes context on why it matters and typical answer options
with tradeoffs.

---

## Section 1: Chain Identity and Purpose

### Q1. What is the chain's primary purpose?

**Why it matters:** Purpose determines every downstream decision -- governance
weight, trust requirements, acceptable work types.

| Option | Tradeoff |
|---|---|
| Open source development | Broad participation, lower trust barriers, public evidence |
| Enterprise coordination | Higher trust requirements, access controls, audit trails |
| Research collaboration | Flexible evidence types, peer review emphasis |
| Education/training | Low stakes, emphasis on learning over output quality |
| Mixed/general | Flexible but harder to optimize governance for any single use |

### Q2. What is the expected scale?

**Why it matters:** Governance overhead scales with participant count.
Manual review works for 10 rigs; at 1000, you need automation.

| Option | Tradeoff |
|---|---|
| Small (< 20 rigs) | Manual governance feasible, personal trust relationships |
| Medium (20-200 rigs) | Need automated trust scoring, delegate review authority |
| Large (200+ rigs) | Require full automation, tiered governance, spider enforcement |

### Q3. Is the chain public or permissioned?

**Why it matters:** Public chains maximize participation; permissioned
chains maximize control and trust baseline.

| Option | Tradeoff |
|---|---|
| Public (anyone can join) | Maximum growth, higher fraud risk, need spider enforcement |
| Permissioned (invitation only) | Higher trust baseline, slower growth, simpler governance |
| Hybrid (public read, permissioned write) | Good balance; browsing is open, but claiming requires approval |

---

## Section 2: Membership and Registration

### Q4. What are the requirements to register as a rig?

**Why it matters:** Registration requirements set the barrier to entry and
influence the trust baseline of all participants.

| Option | Tradeoff |
|---|---|
| DoltHub account only | Lowest barrier, anyone can join, highest fraud exposure |
| DoltHub account + verified email | Modest barrier, ties identity to email provider |
| DoltHub account + existing reputation (e.g., GitHub profile) | Higher signal, excludes brand-new developers |
| Vouched by existing member | Strong trust, creates social graph, limits growth |

### Q5. Are agent rigs allowed?

**Why it matters:** Autonomous agents can scale output but introduce new
trust and quality concerns.

| Option | Tradeoff |
|---|---|
| No agents, humans only | Maximum accountability, slower throughput |
| Agents with human sponsors (`parent_rig` required) | Accountability chain preserved, agents must have a human vouching |
| Unrestricted agents | Maximum throughput, highest risk of automated abuse |

### Q6. What is the initial trust level for new rigs?

**Why it matters:** Starting trust determines what new rigs can do
immediately versus what they must earn.

| Option | Tradeoff |
|---|---|
| Level 0 (unknown) — can browse only | Maximum caution; new rigs must earn even basic participation |
| Level 1 (registered) — can claim and complete | Standard; new rigs can contribute immediately |
| Level 2 (contributor) — can also validate | Fast start but higher risk; unproven rigs validating work |

### Q7. Can rigs be suspended or expelled?

**Why it matters:** Enforcement mechanisms are needed to deal with bad
actors, but overly aggressive enforcement chills participation.

| Option | Tradeoff |
|---|---|
| No expulsion (permanent membership) | Safe harbor, but no recourse against persistent bad actors |
| Suspension by maintainer | Central authority, quick response, single point of failure |
| Suspension by trust-weighted vote | Decentralized, slower response, resistant to capture |
| Automatic suspension by spider rules | Fast, consistent, but can produce false positives |

---

## Section 3: Trust and Reputation

### Q8. How are trust levels computed?

**Why it matters:** Trust is the foundation of the governance model. The
computation method determines how trust is gained, maintained, and lost.

| Option | Tradeoff |
|---|---|
| Manual (maintainer assigns) | Simple, slow, depends on maintainer availability |
| Stamp-based (computed from stamp history) | Automated, transparent, requires sufficient stamp data |
| Hybrid (manual bootstrap, automated maintenance) | Best of both; manual for edge cases, automated for routine |

### Q9. Can trust decay over time?

**Why it matters:** Without decay, inactive rigs retain trust they may no
longer deserve. With decay, active participation is incentivized.

| Option | Tradeoff |
|---|---|
| No decay (trust is permanent) | Simpler, rewards early contributors, risk of stale trust |
| Linear decay (lose trust without activity) | Encourages ongoing participation, penalizes breaks |
| Cliff decay (trust drops after prolonged inactivity) | Tolerates short breaks, clear expectation |

### Q10. What stamp severity levels are used?

**Why it matters:** Severity levels determine the weight of different types
of reputation signals.

| Level | Meaning | Example |
|---|---|---|
| `leaf` | Individual completion assessment | "Good PR, clean code" |
| `branch` | Pattern across multiple completions | "Consistently strong testing" |
| `root` | Fundamental trust determination | "Promoted to trusted reviewer" |

**Options:** Use all three, leaf-only (simple), or leaf+root (skip branch).

### Q11. Is negative stamping allowed?

**Why it matters:** Negative stamps enable accountability but can be used
for harassment or retaliation.

| Option | Tradeoff |
|---|---|
| Positive only | Safe, no retaliation risk, but no way to flag bad work |
| Positive and negative | Full signal, enables accountability, risk of abuse |
| Positive and negative with appeal | Balanced, more complex governance |

---

## Section 4: Work Governance

### Q12. Who can post wanted items?

**Why it matters:** Controls the supply side of the wanted board.

| Option | Tradeoff |
|---|---|
| Anyone (trust level >= 0) | Maximum participation, risk of spam/low-quality items |
| Registered rigs (trust >= 1) | Basic spam prevention |
| Contributors only (trust >= 2) | Higher quality items, limits who can define work |
| Maintainer only | Centralized control, bottleneck risk |

### Q13. Is there a claim limit per rig?

**Why it matters:** Prevents rigs from hoarding work items without
completing them.

| Option | Tradeoff |
|---|---|
| No limit | Maximum flexibility, risk of claim hoarding |
| Fixed limit (e.g., 3 concurrent claims) | Simple, prevents hoarding, may limit productive rigs |
| Trust-scaled limit (higher trust = more claims) | Rewards trusted rigs, more complex |

### Q14. What is the stale claim timeout?

**Why it matters:** Claimed-but-abandoned items block other rigs from
contributing.

| Option | Tradeoff |
|---|---|
| No timeout | Maximum flexibility, risk of permanent blockage |
| 7 days | Standard; enough time for medium-effort work |
| 14 days | Generous; accommodates part-time contributors |
| 30 days | Very generous; for large-effort items only |
| Effort-scaled (small=3d, medium=7d, large=14d) | Precise, more complex to enforce |

### Q15. Who can validate completions?

**Why it matters:** Validation quality determines the integrity of the
entire reputation system.

| Option | Tradeoff |
|---|---|
| Maintainer only | Highest quality, bottleneck, single point of failure |
| Trusted rigs (trust >= 3) | Distributed, scales better, requires trust bootstrapping |
| Any registered rig (trust >= 1) | Maximum throughput, lower validation quality |
| Original poster | The person who posted the item validates; knows requirements best |
| Auto-validate (CI checks pass) | Fastest, only works for items with automated verification |

### Q16. Can a rig validate its own completions?

**Why it matters:** Self-validation is the most basic form of gaming.

| Option | Tradeoff |
|---|---|
| Never (stamps CHECK constraint enforces) | Safe, requires at least two active rigs |
| Only for trust level >= 3 | Pragmatic for small chains, trust-gated |
| Only with automated evidence (CI proof) | Removes human bias, requires CI integration |

---

## Section 5: Fork and Merge Policies

### Q17. What is the merge model?

**Why it matters:** Determines how changes flow from rig forks to the
upstream commons.

| Option | Tradeoff |
|---|---|
| Maintainer merge (manual review) | Highest quality, slowest, bottleneck |
| Auto-merge for trusted rigs (trust >= 3) | Fast for trusted rigs, requires trust bootstrapping |
| Consensus merge (N approvals required) | Distributed authority, slower, resistant to capture |
| Auto-merge all (wild-west mode) | Fastest, lowest quality, Phase 1 default |

### Q18. Is branch protection required?

**Why it matters:** Branch protection prevents accidental or malicious
overwrites of the commons history.

| Option | Tradeoff |
|---|---|
| No protection (current Phase 1) | Simple, fast, risk of history rewriting |
| Main branch protected (PR-only merge) | Standard safety, slight overhead |
| Signed commits required | Strongest integrity, highest setup overhead |

### Q19. How are schema changes governed?

**Why it matters:** Schema changes affect all rigs simultaneously. An
incompatible change can break the federation.

| Option | Tradeoff |
|---|---|
| Maintainer approval only | Safe, single point of control |
| RFC process (propose, discuss, approve) | Transparent, slower, community buy-in |
| Automatic (backward-compatible only) | Fast for additive changes, blocks breaking changes |

---

## Section 6: Dispute Resolution

### Q20. How are disputes about completion quality resolved?

**Why it matters:** Disagreements about whether work meets requirements
are inevitable.

| Option | Tradeoff |
|---|---|
| Maintainer decides | Fast, centralized, single perspective |
| Arbitration panel (3 trusted rigs) | Balanced, slower, requires enough trusted rigs |
| Community vote (trust-weighted) | Most democratic, slowest, risk of popularity contests |
| Re-review with new reviewer | Simple, second opinion, may still disagree |

### Q21. How are disputes about trust levels resolved?

**Why it matters:** Trust level disputes touch identity and reputation,
which are more personal than work quality disputes.

| Option | Tradeoff |
|---|---|
| Maintainer decides | Fast, may feel arbitrary |
| Appeal to arbitration panel | Structured, requires process overhead |
| Automatic (stamp history is the record) | Objective, no recourse for edge cases |

### Q22. Is there a dispute cooldown period?

**Why it matters:** Prevents dispute flooding and gives parties time to
reflect.

| Option | Tradeoff |
|---|---|
| No cooldown | Maximum responsiveness, risk of flame wars |
| 48-hour cooldown between dispute filings | Encourages reflection, delays resolution |
| 7-day cooldown | Strong cooling-off period, may frustrate urgent disputes |

---

## Section 7: Economics and Incentives

### Q23. Does the chain have an economic model?

**Why it matters:** Economic incentives can drive participation but add
complexity and potential for gaming.

| Option | Tradeoff |
|---|---|
| No economics (reputation only) | Simple, focuses on intrinsic motivation |
| Bounty system (items have monetary value) | Strong incentive, attracts mercenaries, complexity |
| Token-based (chain-specific credits) | Internal economy, requires token design, liquidity |

### Q24. Are there incentives for validation work?

**Why it matters:** Validation is essential but unglamorous. Without
incentives, rigs prefer completing over reviewing.

| Option | Tradeoff |
|---|---|
| No incentive (altruistic validation) | Simple, may lead to validation backlog |
| Stamps count toward trust (validates earn reputation) | Natural incentive, aligns with trust model |
| Explicit reviewer rewards | Strong incentive, risk of rubber-stamp reviews |

### Q25. Is there a penalty for abandoned claims?

**Why it matters:** Rigs that claim and abandon waste others' time and
block items.

| Option | Tradeoff |
|---|---|
| No penalty (item returns to open) | Lenient, no disincentive against speculative claims |
| Trust deduction on abandon | Strong disincentive, may discourage risky claims |
| Cooldown before next claim | Mild penalty, limits throughput without trust impact |

---

## Section 8: Evolution and Maintenance

### Q26. How often should spiders run?

**Why it matters:** Spider frequency determines how quickly anomalies are
detected and addressed.

| Option | Tradeoff |
|---|---|
| Hourly | Fast detection, higher resource usage |
| Daily | Good balance for most chains |
| Weekly | Low overhead, slower detection |
| On-demand only | Minimal resources, requires manual trigger |

### Q27. What triggers a constitution amendment?

**Why it matters:** Constitutions must evolve, but frequent changes
destabilize expectations.

| Option | Tradeoff |
|---|---|
| Maintainer unilateral | Fast, centralized, may surprise participants |
| Proposal + 7-day comment period + maintainer approval | Transparent, gives notice |
| Supermajority vote of trusted rigs | Democratic, slow, requires quorum |

### Q28. What is the chain's data retention policy?

**Why it matters:** Long retention provides complete history; short
retention reduces storage and privacy exposure.

| Option | Tradeoff |
|---|---|
| Permanent (all history retained) | Complete audit trail, growing storage |
| Rolling window (e.g., 2 years) | Bounded storage, loses historical context |
| Tiered (active data hot, old data archived) | Balanced, more complex infrastructure |

---

## Example: Open Source Developer Chain

Below is a filled-out questionnaire for a hypothetical chain called
"oss-dev" focused on open source development coordination.

| # | Question | Answer |
|---|---|---|
| Q1 | Purpose | Open source development |
| Q2 | Scale | Medium (20-200 rigs) |
| Q3 | Public/permissioned | Hybrid (public read, permissioned write) |
| Q4 | Registration requirements | DoltHub account + GitHub profile |
| Q5 | Agent rigs | Agents allowed with human sponsors |
| Q6 | Initial trust level | Level 1 (registered) |
| Q7 | Suspension/expulsion | Suspension by maintainer + appeal process |
| Q8 | Trust computation | Hybrid (manual bootstrap, stamp-based maintenance) |
| Q9 | Trust decay | Cliff decay (drops after 90 days inactivity) |
| Q10 | Stamp severity levels | All three (leaf, branch, root) |
| Q11 | Negative stamping | Positive and negative with appeal |
| Q12 | Who can post wanted items | Registered rigs (trust >= 1) |
| Q13 | Claim limit | Trust-scaled (trust 1: 2, trust 2: 5, trust 3: 10) |
| Q14 | Stale claim timeout | Effort-scaled (small=3d, medium=7d, large=14d) |
| Q15 | Who validates | Trusted rigs (trust >= 3) + original poster |
| Q16 | Self-validation | Never |
| Q17 | Merge model | Auto-merge for trust >= 3, maintainer review for others |
| Q18 | Branch protection | Main branch protected (PR-only) |
| Q19 | Schema governance | RFC process |
| Q20 | Completion disputes | Re-review with new reviewer, escalate to maintainer |
| Q21 | Trust disputes | Appeal to arbitration panel (3 trusted rigs) |
| Q22 | Dispute cooldown | 48 hours |
| Q23 | Economic model | No economics (reputation only) |
| Q24 | Validation incentives | Stamps count toward trust |
| Q25 | Abandon penalty | Cooldown (24h before next claim) |
| Q26 | Spider frequency | Daily |
| Q27 | Constitution amendments | Proposal + 7-day comment + maintainer approval |
| Q28 | Data retention | Permanent |

### Rationale

This configuration balances openness with quality:

- **Hybrid access** lets anyone discover work but gates contributions behind
  registration, reducing drive-by spam.
- **Agent support with sponsors** enables automation while preserving
  accountability chains.
- **Effort-scaled timeouts** prevent claim hoarding without penalizing
  legitimate large-effort work.
- **Reputation-only economics** keeps the focus on intrinsic motivation and
  avoids the complexity of bounty systems in early phases.
- **Daily spiders** catch anomalies within 24 hours without excessive
  resource usage.

---

## Next Steps

After completing this questionnaire, the answers feed into:

1. **Constitution generation:** A structured document (or database record)
   encoding the chosen parameters
2. **Spider configuration:** Spider queries tuned to the chain's thresholds
   (see [Spider Protocol Queries](spider-protocol-queries.md))
3. **Herald configuration:** Notification rules matching the governance model
4. **Chain launch:** Fork the commons template, apply the constitution, and
   announce to the federation
