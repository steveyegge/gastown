# Medici Collision: Agent UX × Adversary

**Issue:** gt-4kw — Rally Tavern integration design review

## The Tension

**Agent UX says:** Push knowledge automatically into agent context at prime time. Don't make agents pull — they won't. Auto-injection maximizes adoption and closes the "advisory steps get skipped" gap.

**Adversary says:** Auto-injecting knowledge amplifies the blast radius of wrong entries. A pull model (agent opts in) limits damage because agents who don't search never encounter bad knowledge. Push means every agent sees every error.

This is the classic reach-vs-safety tradeoff in any content distribution system.

## What Each Side Is Correctly Seeing

**Agent UX is right that:** Pull-based knowledge systems die from neglect. Advisory formula steps get skipped. If Rally Tavern knowledge isn't automatically visible, it won't be used, and if it's not used, no one will contribute, and the whole system atrophies. The integration only has value if agents actually consume the knowledge.

**Adversary is right that:** Auto-injection without quality guarantees turns Rally Tavern into a liability. One wrong entry that gets auto-injected into every polecat's prime context will cause systematic bad decisions at scale. The damage is proportional to reach — and push maximizes reach.

## What Each Side Is Missing

**Agent UX misses:** That auto-injection creates a quality SLA. If knowledge appears automatically in primed context, users (human and agent) will expect it to be *correct*. The barkeep's current "lean toward accepting with light editing" stance isn't compatible with auto-injection — acceptance standards must be higher when distribution is automatic.

**Adversary misses:** That the *absence* of knowledge also causes harm. Agents without Rally Tavern knowledge repeat mistakes, rediscover known solutions, and produce inconsistent patterns. The adversary lens focuses on the harm of wrong knowledge but ignores the ongoing cost of no knowledge. At current corpus quality (curated by barkeep with quality guidelines), the expected value of auto-injection is positive.

## Recommendation

**Graduated injection based on verification status.**

1. **Verified entries (last_verified within 60 days):** Auto-inject at prime time. These have been confirmed accurate by at least one agent recently. The blast radius of wrong knowledge is small because verification is recent.

2. **Unverified entries (never verified, or last_verified >60 days):** Available via `gt rally search` but NOT auto-injected. Agents who actively search take on the responsibility of evaluating relevance. This preserves the pull model for unverified content.

3. **New entries (<7 days old, never verified):** Auto-inject with a `[NEW — unverified]` tag. New knowledge is likely accurate (just reviewed by barkeep) and most useful when fresh. After 7 days without verification, it drops to category 2.

This gives agent-ux the auto-injection it needs for adoption while giving adversary the quality gate it needs for safety. The verification timestamp becomes the bridge between push and pull.

**Implementation cost:** Modify `gt prime`'s knowledge injection to filter by `last_verified` date. Modify `score()` to include a verification boost. Add a `Verified() bool` method to `KnowledgeEntry`. ~20 lines of Go.

## Residual Risk

The graduated model still depends on agents running `gt rally verify` to keep entries in the auto-inject tier. If verification doesn't happen (the incentives lens flags this risk), entries will drift from tier 1 (auto-inject) to tier 2 (pull-only) and the auto-inject pool will shrink over time. The barkeep patrol should include a "re-verify oldest entries" step to prevent this decay.
