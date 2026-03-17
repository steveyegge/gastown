# Medici Lens: Agent UX

**Issue:** gt-4kw — Rally Tavern ↔ Gas Town integration design review
**Bead:** gt-raq

## Framing

From a working polecat's perspective, Rally Tavern knowledge is a *context injection* — extra tokens added to the agent's working memory during planning or implementation. The fundamental UX question isn't "can the agent search?" but "does the search result justify the tokens it costs?"

Every `gt rally search` result that appears in a polecat's formula step consumes context window space. If the result is irrelevant, it's worse than nothing — it's noise that displaces useful context. The integration is advisory (agents can skip it), but advisory features that consistently waste tokens get mentally flagged as "skip this" and never recover.

## What Other Lenses Will Likely Miss

**The formula integration is the critical UX touchpoint, not the CLI.**

The `gt rally search` CLI is fine — agents can call it, read results, and move on. But the real adoption surface is the formula steps (mol-polecat-work, mol-idea-to-plan) where knowledge lookup is embedded in the workflow.

Currently, the plan says to add instructions like "search for relevant knowledge and include it if useful." But this is exactly the kind of optional, vague instruction that agents optimize away under token pressure. The step doesn't have a gate (no exit criteria tied to knowledge), so agents will read it, decide "probably nothing relevant," and skip.

The core insight: **knowledge injection should be automatic and invisible, not a manual step the agent decides to take.** If the knowledge is relevant, it should already be in the agent's context at prime time — not something they go fetch mid-implementation.

## Proposed Solution

**Auto-inject relevant knowledge at `gt prime` time, not as a formula step.**

1. When `gt prime` runs, if rally_tavern is available, automatically run `gt rally search --profile --limit=3 --json` against the current repo's tavern-profile.yaml
2. Append the top 3 results (titles + summaries only, ~50 tokens each) to the primed context under a `## Rally Tavern Knowledge` section
3. Remove the manual "search for knowledge" instructions from formulas — they become redundant
4. Keep `gt rally search` and `gt rally lookup` as opt-in CLI tools for agents who want to dig deeper

This changes the integration from "pull" (agent decides to search) to "push" (knowledge appears automatically if relevant). Push beats pull for agent adoption every time.

## Failure Mode

**Signal-to-noise death spiral.** If the knowledge corpus is thin (early days: <20 entries), most auto-searches return nothing or irrelevant results. Agents see "No results found" or low-quality matches repeatedly. The knowledge section in primed context becomes a known dead zone — agents learn to ignore it even when good entries appear later. The first impression sets the adoption trajectory.

## Cheap Experiment

**Manually test the current search against 5 real bead descriptions.** Take the last 5 completed beads from any rig, extract keywords from their titles/descriptions, and run `gt rally search "<keywords>"` against the current knowledge corpus. Measure: (a) how many return ≥1 result, (b) how many of those results are actually relevant. If it's <40% useful hit rate, the search isn't ready for auto-injection and needs corpus growth first.
