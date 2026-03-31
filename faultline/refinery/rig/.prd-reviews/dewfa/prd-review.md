# PRD Review: Faultline — Self-hosted Sentry-compatible Error Tracker

## Executive Summary

The faultline PRD (README.md) communicates a compelling vision — Sentry-compatible ingest, Dolt-native storage with time-travel, and a Gas Town agentic auto-fix loop — but operates at pitch-deck level, not specification level. P1 implementation is solid and exceeds what the PRD describes, but every subsequent phase has undefined boundaries, no success criteria, and no interface contracts. The two highest risks are: (1) "Sentry SDK compatible" is an unbounded scope claim with no compatibility matrix, and (2) the Gas Town integration (P4) — the project's core differentiator — has zero specification. Without resolving these, different engineers would build materially different systems.

## Before You Build: Critical Questions

These must be answered before P2+ implementation begins.

### Requirements / Scope

**Q1: Which Sentry SDK features are in scope?**
- Why this matters: "Drop-in replacement for any Sentry SDK" could mean supporting 30+ platform SDKs with sessions, transactions, replays, profiles, check-ins, attachments, and source maps — or it could mean Python/JS/Go error events only. The current implementation handles `event`, `transaction`, and `error` item types and skips everything else. Without an explicit compatibility matrix, scope is unbounded and "done" is undefined.
- Found by: ambiguity, feasibility, gaps, requirements, scope, stakeholders (all 6 legs)
- Suggested answer: Define an SDK compatibility matrix — which SDKs, which versions, which event types (error only? transaction? session?). Add an explicit "Out of Scope" section listing features not planned (session replay, profiling, source maps, cron monitors, etc.).

**Q2: What is the Dolt commit granularity?**
- Why this matters: "Every error event is a git commit" is the core Dolt value proposition but has massive architectural implications. Per-event commits give true time-travel but won't scale (Dolt commits are expensive Prolly-tree merges). Batch commits give performance but weaken the narrative. The current implementation does standard SQL INSERT with no `CALL dolt_commit()` at all — so the claim may already be false.
- Found by: ambiguity, feasibility, gaps, scope (4 legs)
- Suggested answer options: (a) Per-event commits (strong narrative, poor perf), (b) Periodic batch commits (practical, weaker time-travel), (c) On-demand/manual commits (defers the question), (d) No Dolt commits for now, revisit in P5.

**Q3: What is the Gas Town integration contract?**
- Why this matters: This is the entire P4 phase and the project's raison d'etre. Without specification, P1-P3 is "worse Sentry with fewer features." Every arrow in the loop diagram (`error → bead → polecat → fix → merge → resolved → watch regression`) is an unspecified interface.
- Found by: ambiguity, requirements, scope, gaps, stakeholders (5 legs)
- Suggested answer: Specify these five contracts:
  1. **Bead creation trigger**: Every new issue group? Above event threshold? Manual?
  2. **Bead content schema**: What data does the polecat receive?
  3. **Polecat assignment**: Which rig? Which formula?
  4. **Resolution detection**: How does faultline know a PR fixed a specific group?
  5. **Failure path**: What if the polecat can't fix it?

**Q4: What does "regression" mean?**
- Why this matters: Could be trivial (same fingerprint reappears after close = reopen) or a multi-sprint feature (statistical regression detection with time windows and thresholds). Sentry considers this one of their most complex features.
- Found by: ambiguity, gaps, scope (3 legs)
- Suggested answer options: (a) Same fingerprint after resolution = reopen bead, (b) Defer entirely to post-P5, (c) Full state machine (resolved → regressed → archived) with configurable windows.

**Q5: What is the target event volume?**
- Why this matters: The architecture is fundamentally different at 1 ev/s (side project) vs 100 ev/s (small team) vs 1000+ ev/s (production). The current design (single Go process, synchronous DB writes, no queue, no batching, no connection pooling) implies low volume, but "Sentry compatible" implies production use.
- Found by: feasibility, requirements, scope (3 legs)
- Suggested answer: Pick a target range and validate Dolt can sustain it. This determines whether queuing, batching, and connection pooling are in-scope work or premature optimization.

### Feasibility / Architecture

**Q6: How will bead storms be prevented?**
- Why this matters: During a deploy gone wrong, one root cause can produce dozens of distinct fingerprints. Without deduplication/cooldown at the error→bead boundary, one bad deploy could spawn 50+ polecats working on the same underlying bug — taxing Gas Town's already-fragile Dolt server.
- Found by: feasibility (1 leg, but high severity)
- Suggested answer options: (a) Time-based cooldown on bead creation, (b) Similarity threshold across fingerprints, (c) Manual triage gate before bead creation, (d) Rate limit on bead creation per project per time window.

**Q7: What are the phase boundaries relative to current state?**
- Why this matters: P2 is described as "Dolt schema + issue grouping + fingerprinting" but P1 already implements all three. Engineers can't estimate remaining work or plan when phases overlap with shipped code.
- Found by: ambiguity, requirements, scope (3 legs)
- Suggested answer: Redefine each phase as a delta from current state. What does each phase ADD that doesn't exist yet?

## Important But Non-Blocking

Implementation can start on the read API and schema refinements, but these need resolution before their respective phases ship.

- **Dashboard scope (P3)**: "React/Vite dashboard" could be a single status page or a full Sentry-like UI. Define minimum screens: issue list, event detail, stack trace viewer? Actions (resolve, ignore, assign)? Charts/trends? This is potentially the largest phase by engineering effort. (Found by: ambiguity, scope, stakeholders)

- **Authentication model for the query/dashboard API**: The ingest API uses sentry_key (write-only). The dashboard needs a different auth model (user sessions, RBAC). This needs design before P3 ships. (Found by: gaps, requirements, stakeholders)

- **Data retention and storage management**: Raw JSON events accumulate indefinitely. No TTL, archival, compaction, or pruning is defined. Dolt's append-only commit history makes this especially acute. Needs a policy before significant data accumulates. (Found by: gaps, scope, stakeholders)

- **Rate limiting and backpressure**: No rate limiting per project, per IP, or globally. SDKs expect `429` with `Retry-After` headers. Without this, a misconfigured SDK or bad deploy floods the system. Essential for SDK compatibility, not just operational safety. (Found by: feasibility, gaps)

- **Transaction safety in ingest path**: `UpsertIssueGroup` and `InsertEvent` are separate operations — a crash between them leaves inconsistent state. Wrap in a single Dolt transaction. (Found by: feasibility)

- **No connection pooling or liveness checks**: Single connection via `sql.Open` with no pool config. Under load, unbounded connections could overwhelm Dolt. Set `MaxOpenConns`/`MaxIdleConns` and add DB-aware health check. (Found by: feasibility)

- **Multi-tenancy model**: Projects exist but there's no concept of organizations, teams, or access scoping. Is faultline single-tenant (one deployment per org) or multi-tenant? Determines auth model, data isolation, and API design. (Found by: gaps, scope, stakeholders)

- **Security / PII in error payloads**: Raw events may contain credentials, PII, sensitive paths. No scrubbing, no access controls, no retention policy. Compliance liability for teams subject to data protection regulations. (Found by: stakeholders, gaps)

- **Operations / self-monitoring**: No metrics, no alerting, no self-monitoring. The `/health` endpoint doesn't check DB connectivity. An error tracker with no error tracking of itself. (Found by: stakeholders, gaps, requirements)

## Observations and Suggestions

- **Implementation exceeds PRD quality**: P1 codebase is more thoughtful than the PRD warrants — idempotent inserts, gzip support, multi-format timestamps, graceful shutdown, minimal dependencies. The engineering is solid; the spec needs to catch up.

- **Fingerprinting is well-done but has known fragility**: SHA256 priority chain matches Sentry's grouping logic. However, `truncate()` operates on bytes not runes (can split UTF-8), and parameterized messages ("failed after 3 retries" vs "5 retries") produce different fingerprints. Worth noting for P2 fingerprint quality improvements.

- **"Zero extra infrastructure" is aspirational**: Faultline requires Go server + Dolt server + eventually React frontend. For users without Dolt, faultline IS significant infrastructure. Qualify the claim or acknowledge prerequisites.

- **Source maps are conspicuously absent**: Any JavaScript SDK integration will immediately generate requests for source map upload and server-side deobfuscation. If JS SDKs are in scope, source maps follow within days of launch.

- **The loop diagram implies synchronous flow but the system is async**: The arrow from "polecat fixes → faultline marks resolved" implies a callback from the merge system to faultline. No such integration exists or is described.

- **Dolt `dolt_commit` integration is missing from P1**: Current code does standard SQL inserts that land in the working set but aren't committed to Dolt's version history. P5 (time-travel, fingerprint branching) has no foundation without this.

- **No deployment/packaging story**: How is faultline distributed? Docker image? Go binary? Homebrew? Affects configuration, TLS, and operational assumptions.

- **P6 (myndy_ios dogfooding) mentioned in arch-review-input but not in README**: If real, implies mobile-specific requirements (crash reporting, symbolication, dSYM handling) that fundamentally differ from server-side error tracking.

## Confidence Assessment

| Dimension | Score | Notes |
|-----------|-------|-------|
| Requirements completeness | **L** | PRD is a pitch deck, not a spec. No success criteria, no acceptance conditions, no testable requirements for any phase. |
| Technical feasibility | **M-H** | P1 proves core ingest works. P2-P3 are straightforward extensions. P4-P5 carry meaningful risk around Dolt write throughput and bead storm prevention. |
| Scope clarity | **L** | No out-of-scope statements. "Sentry SDK compatible" is unbounded. Phase boundaries don't match shipped code. Dashboard is a scope black hole. |
| Ambiguity level | **H** (high ambiguity) | Two engineers would agree on direction but diverge sharply on scope, compatibility boundaries, and integration semantics. |
| Overall readiness | **L** | Buildable through P1 (done). P2 possible with assumptions. P3+ requires answers to critical questions above. |

## Next Steps

- [ ] Human answers critical questions above (Q1-Q7)
- [ ] Update PRD with answers, compatibility matrix, and out-of-scope section
- [ ] Redefine phase boundaries as deltas from current state
- [ ] Write P4 specification (Gas Town integration contract)
- [ ] Pour `design` convoy to generate implementation plan
