# Medici Lens: Adversary

**Issue:** gt-4kw — Rally Tavern ↔ Gas Town integration design review
**Bead:** gt-4od

## Framing

The most dangerous failure mode of a knowledge system isn't absence of knowledge — it's *wrong* knowledge that agents trust. A polecat that finds nothing in Rally Tavern does its own research. A polecat that finds a confident-sounding but outdated practice applies it without question, producing code that follows an obsolete pattern.

The design has `deprecated` and `superseded_by` fields, plus `gt rally report` for flagging stale/wrong entries. But these are reactive mechanisms — they depend on an agent recognizing that knowledge it just applied was wrong, which usually happens *after* the damage (bad code committed, wrong pattern established).

## What Other Lenses Will Likely Miss

**The verification loop has no teeth.**

The `last_verified` field and `gt rally verify` command are elegant — agents can confirm entries are still accurate. But there's no mechanism to *require* verification, and no consequence for an unverified entry. An entry from 6 months ago with `last_verified: ""` surfaces in search results with the same rank as a freshly verified entry.

The search scoring function in `knowledge.go` doesn't consider `last_verified` or `created_at` at all. A practice written for a codebase version that no longer exists ranks the same as one verified yesterday. Time decay is absent from the relevance model.

Worse: the barkeep's quality gate operates at *acceptance time*, not at *consumption time*. An entry that was correct when accepted can silently rot as the codebase evolves. There's no automated check that knowledge entries are still applicable.

## Proposed Solution

**Add time decay to search scoring and automated staleness alerts.**

1. **Time decay in search:** Modify the `score()` function to penalize entries where `created_at` is >90 days old AND `last_verified` is empty or >60 days old. This doesn't remove old entries — it ranks fresher/verified entries higher.

```
if entry.CreatedAt > 90d ago && entry.LastVerified > 60d ago:
    score -= 1  // soft penalty, still surfaces but ranked lower
```

2. **Staleness alert in barkeep patrol:** During each patrol, the barkeep scans for entries where `last_verified` is >90 days old or empty. For each, it creates a lightweight verification request: "Is this still accurate?" These could be attached to the next polecat that works on a bead with matching tags.

3. **Negative feedback path:** When an agent finds a knowledge entry unhelpful or wrong *during implementation*, make `gt rally report --kind=wrong` available as a one-liner with no required fields beyond entry-id. The current implementation requires `reason` for stale/wrong — removing this for a quick negative signal reduces friction.

## Failure Mode

**Confident-wrong cascade.** A knowledge entry about Dolt session management says "always use `gt dolt sql -e` for queries." This was correct when written but the command was removed in a later version. Three polecats apply this advice, get errors, waste time debugging, and file desire-path beads. None of them think to run `gt rally report` because they assume the error is in their own code, not in the knowledge base. The wrong entry continues to rank high in searches because it has good tag matches.

This is the second-order harm: wrong knowledge doesn't just fail once — it fails repeatedly and silently, training agents to distrust the system.

## Cheap Experiment

**Plant a deliberately outdated entry and measure detection time.** Add a knowledge entry with a known-wrong instruction (e.g., "use flag `--legacy` with gt prime"). Monitor how many agents encounter it, how many report it, and how long until it gets flagged. This measures the natural immune response of the system to knowledge rot. If detection takes >1 week, the staleness mechanisms need strengthening.
