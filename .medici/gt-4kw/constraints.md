# Medici Lens: Constraints

**Issue:** gt-4kw — Rally Tavern ↔ Gas Town integration design review
**Bead:** gt-xnf

## Framing

The design makes several "for now" choices that work at current scale but encode operational assumptions:

1. **In-memory index, no caching:** Every `gt rally search` invocation reads and parses all YAML files from disk. At 20-50 files, this takes milliseconds. At 500 files, it could take hundreds of milliseconds — noticeable in agent workflows.

2. **`$GT_ROOT/rally_tavern/` must exist on the filesystem:** The graceful degradation is good (nil, nil return), but the *path* is hardcoded. In CI environments, containerized rigs, or new dev setups, rally_tavern may not be cloned. The integration silently becomes a no-op.

3. **Dolt dependency for nominations:** The mail system (nomination transport) runs on Dolt. If Dolt is unhealthy (a known recurring issue per CLAUDE.md), nominations queue up in broken state and never reach the barkeep. The contribution loop has a single point of failure.

4. **YAML-only knowledge format:** Every entry is a YAML file. No binary assets, no code snippets longer than what fits in a YAML string, no links to external resources beyond what's in the text.

## What Other Lenses Will Likely Miss

**The filesystem coupling is the binding constraint, not the search algorithm.**

Other lenses will focus on search quality, adoption, or routing. But the hardest constraint is that the entire system depends on a specific directory layout at a specific path on a specific machine. This works for a single-developer setup with everything co-located, but it rules out:
- Remote/distributed Gas Town instances
- CI/CD environments that don't clone rally_tavern
- Multiple Gas Town installations sharing a knowledge base
- Testing the integration without a real rally_tavern checkout

The `LoadKnowledgeIndex` function takes `gtRoot string` as a parameter (good for testing), but the CLI command resolves this from `workspace.FindFromCwdOrError()` — which walks up the directory tree to find the GT root. There's no fallback (like a remote HTTP endpoint or a cached snapshot).

## Proposed Solution

**Add a knowledge snapshot mechanism that decouples search from filesystem presence.**

1. **`gt rally snapshot`**: New command that serializes the current knowledge index to a single JSON file (`.cache/rally-knowledge.json`) in the rig's working directory. This runs during `gt dolt backup` or on a schedule.
2. **Fallback in `LoadKnowledgeIndex`**: If `$GT_ROOT/rally_tavern/` doesn't exist, check for `.cache/rally-knowledge.json` in the repo root. If it exists, load from that instead.
3. This enables: CI environments (snapshot checked into repo or generated in setup), distributed setups (snapshot synced via git), and Dolt-down resilience (search works from snapshot even if nominations are blocked).

Keep the live filesystem as the primary source; snapshot is a read-only fallback.

## Failure Mode

**Dolt-down nomination blackhole.** Dolt server goes unhealthy (connection refused, slow queries, orphan databases — all documented recurring issues). Agent finishes a bead, runs `gt rally nominate`, which sends mail via Dolt. The mail write fails silently or hangs. The agent's session continues (nomination is advisory), but the knowledge is lost. The agent will never nominate the same thing again in a future session.

This is an invisible failure: no error shown to the agent, no retry mechanism, no dead-letter queue. The nomination simply vanishes. Over time, this erodes the corpus as contributions are silently lost during Dolt instability windows.

## Cheap Experiment

**Inject a Dolt-down simulation during nomination.** Temporarily stop Dolt, run `gt rally nominate` with valid input, and observe: (a) does the command error gracefully? (b) is the nomination preserved anywhere for retry? (c) does the agent's session recover? If the answer to (b) is "no," add a local file fallback (write nomination YAML to `.cache/pending-nominations/`) that gets flushed to mail when Dolt recovers.
