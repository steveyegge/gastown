# Medici Lens: Domain

**Issue:** gt-4kw — Rally Tavern ↔ Gas Town integration design review
**Bead:** gt-0ri

## Framing

The knowledge model treats agent knowledge as a library catalog: structured YAML entries with typed categories (practice/solution/learned), tag-based indexing, and substring search. But agent knowledge is more like a conversation — messy, contextual, and often impossible to cleanly categorize.

The current `KnowledgeEntry` struct forces a rigid category split: practices have gotchas/examples, solutions have problem/solution, learned has context/lesson. But real agent discoveries blend these — "I learned that Dolt flattening timing matters" is simultaneously a practice, a solution, and a lesson. The category becomes arbitrary, and searchers must know which category an entry was filed under to predict where to find it.

## What Other Lenses Will Likely Miss

**The taxonomy is the bottleneck, not the search algorithm.** Other lenses will focus on search quality, adoption friction, or routing topology. But the most fundamental issue is that the three-category split (practices/solutions/learned) is a classification scheme imposed at write-time that may not match how agents think at read-time. An agent looking for "how to handle Dolt connections" doesn't know if that's a practice, solution, or lesson — they just need the answer.

The search does span all categories (good), but the category split affects:
- How nominators think about what to nominate ("is this a practice or a lesson?")
- How the barkeep evaluates quality (different expectations per category)
- How the corpus organizes on disk (separate directories)

## Proposed Solution

**Flatten the category distinction for search; keep it for curation only.**

1. Remove the category field from `SearchQuery` (it's not there now — good)
2. Add a `SearchQuery.Fields` option to restrict which fields get text-matched (currently hardcoded)
3. Consider a unified `knowledge/` directory with category as a YAML field, not a directory split — this eliminates the routing decision at nomination time
4. Add a `related_to` field in `KnowledgeEntry` that links to other entries by ID, enabling "see also" chains that cross categories

## Failure Mode

**Category paralysis at nomination time.** An agent finishes a bead, has a useful insight, and the nomination prompt asks "is this a practice, solution, or learned?" The agent hesitates, picks wrong, or skips nominating entirely. The contribution funnel leaks at the classification step.

Evidence: `Nomination.Validate()` requires a valid category. If the agent picks "practice" but the barkeep thinks it's "learned", there's friction in the acceptance path. The barkeep might edit it, but that's invisible reclassification that the nominator never learns from.

## Cheap Experiment

**Log category distribution for the first 20 nominations.** If the split isn't roughly even (say >60% in one category), the taxonomy isn't capturing natural variation — it's just a default bucket. Add a `--category=auto` flag to `gt rally nominate` that uses heuristics (has gotchas? → practice. has problem/solution pair? → solution. default → learned) to auto-classify and compare with agent choices.
