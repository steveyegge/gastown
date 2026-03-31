# Scope Analysis

## Summary

The faultline PRD defines a five-phase project that spans from Sentry-compatible ingest (P1, largely built) through a fully autonomous error-to-fix agentic loop (P4) and Dolt-native time-travel features (P5). The scope is ambitious but critically underspecified at the boundaries: "Sentry SDK compatible" has no compatibility matrix, the phase breakdown doesn't reflect what's already shipped, and the project's core differentiator (Gas Town auto-fix) has no contract, trigger model, or failure semantics. The PRD lacks explicit out-of-scope statements entirely, which means every adjacent Sentry feature is implicitly in-scope until someone says otherwise. The highest scope creep risks are in SDK compatibility breadth, dashboard feature expectations, and the Gas Town integration contract — each of which could individually double the project's effort.

There is a strong tension between the "zero extra infrastructure" positioning and the reality that faultline is a distributed system (Go server + Dolt + Gas Town + dashboard). The MVP needs sharper definition: the smallest version that delivers value is probably "ingest + read API + bead creation" (P1-P3 minus dashboard), but the PRD doesn't distinguish must-haves from nice-to-haves within or across phases.

## Findings

### Critical Gaps / Questions

- **No explicit out-of-scope statements exist anywhere in the PRD.** The README is purely additive — it says what faultline does but never says what it doesn't do. This is the single biggest scope risk. Without boundaries, any Sentry feature is a reasonable request: session replay ingest, release health tracking, performance monitoring, cron monitoring, user feedback, attachments, source maps, and dozens more. Each of these is a multi-week effort.
  - Why this matters: Engineers and stakeholders will have different mental models of what "Sentry compatible" means. Without explicit exclusions, scope negotiations happen at implementation time rather than planning time.
  - Suggested question: Add an "Out of Scope" section listing at minimum: session replay, performance tracing (beyond basic transaction ingest), release health, cron monitors, user feedback, profiling, source maps, and any other Sentry features not planned. State whether "not in any phase" or "deferred beyond P5."

- **"Sentry SDK compatible — drop-in replacement for any Sentry SDK" is an unbounded scope claim.** Sentry supports 30+ platform SDKs with different envelope schemas, feature negotiation, and data formats. The current implementation handles `event`, `transaction`, and `error` item types and skips everything else. "Drop-in replacement" implies zero client-side changes, but the DSN provisioning, feature discovery, and SDK handshake are all unspecified. As the ambiguity analysis also flagged: this is the #1 scope risk.
  - Why this matters: Without a compatibility matrix, the definition of "done" for P1 (and the entire project) is subjective. One reviewer might say P1 is complete today; another might say it needs session, attachment, and profile support.
  - Suggested question: Define an explicit SDK compatibility matrix: which SDKs (Python? JS? Go?), which versions, which event types (error only? transaction? session?), which features (breadcrumbs? contexts? tags? user?). "Any SDK" is not a shippable requirement.

- **Phase boundaries don't reflect current implementation state.** P1 is described as "Sentry SDK ingest (envelope + legacy store)" and P2 as "Dolt schema + issue grouping + fingerprinting." But P1's implementation already includes: Dolt schema (2 tables with migrations), issue grouping (group_hash upsert with event_count), and fingerprinting (SHA256, 3-tier priority). P2 as described is already shipped. This creates confusion about what work remains.
  - Why this matters: Engineers can't estimate remaining effort when phase descriptions overlap with completed work. Stakeholders can't track progress against phases that are partially done before they officially start.
  - Suggested question: Redefine phases as deltas from current state. What does each phase ADD that doesn't exist yet? Consider reframing: P2 might be "Dolt-native commit semantics + advanced fingerprinting" if the basic schema/grouping is already P1.

- **The Gas Town integration (P4) — the core differentiator — has no specification.** The loop diagram shows `error → bead → polecat → fix → merge → resolved`, but none of these transitions are defined:
  - **Trigger**: What creates a bead? Every new issue group? Only groups above an event threshold? Only specific severity levels? Manual trigger?
  - **Content**: What goes in the bead? Stack trace? Repo path? Reproduction steps? How does the polecat know which codebase to look at?
  - **Assignment**: How is a polecat selected? Is this a new molecule formula?
  - **Resolution**: How does faultline know a PR fixed a specific error group? Commit message convention? API callback? Polling?
  - **Failure**: What if the polecat can't fix it? What if the fix introduces new errors?
  - Why this matters: P1-P3 without P4 is "worse Sentry with fewer features." The Gas Town loop IS the value proposition. Without specification, P4 is either a weekend project (simple bead creation) or a multi-month effort (full bidirectional integration with resolution tracking). The scope difference is 10x.
  - Suggested question: Write a P4 specification covering: bead creation trigger, bead content schema, polecat assignment strategy, resolution detection mechanism, and failure/escalation paths.

- **"Watches for regression" is completely undefined.** This phrase appears in the loop diagram but has no specification. Regression detection in Sentry is a complex state machine (resolved → regressed → archived, with configurable windows and thresholds). It could mean: same fingerprint reappears after close, same exception type from same code path, statistical increase in error rate, or manual re-open.
  - Why this matters: This is either a 1-line check (same hash after close = reopen) or a multi-sprint feature (statistical regression with time windows). It needs to be scoped explicitly or deferred.
  - Suggested question: Define "regression" precisely. Is it fingerprint re-occurrence after resolution? Within what time window? What action is taken — reopen bead, create new bead, alert only? Or defer this entirely to a later phase?

### Important Considerations

- **The dashboard (P3) is a scope black hole.** "React/Vite (dashboard)" could mean a single-page status view or a full Sentry-like UI with issue lists, event detail, stack trace rendering, graphs, search, filters, real-time streaming, and admin controls. Sentry's UI is the product of years of engineering. Even a minimal dashboard (issue list + event detail + stack trace) is a significant frontend effort that dwarfs the backend work.
  - Suggested question: Define dashboard screens explicitly. Minimum: issue list, event detail, stack trace viewer. What about: charts/trends, search/filter, resolve/ignore actions, project settings, user management? Consider whether a CLI or API-only interface is the MVP and dashboard is a separate phase.

- **Dependencies on other teams/systems are not scoped.** The Gas Town integration requires changes to: the beads system (new bead type or creation trigger), the witness (detection of new faultline beads), the molecule system (new formula for error-fixing polecats), and potentially the refinery (resolution callback). None of these are called out as cross-team dependencies.
  - Suggested question: Which Gas Town components need modification for P4? Who owns those changes? Are they in scope for this project or separate projects?

- **"Zero extra infrastructure" is aspirational, not accurate.** Faultline requires: a Go HTTP server, a Dolt server (which the user may or may not already run), and eventually a React frontend build/serve pipeline. For users who don't already run Dolt, faultline IS significant infrastructure. The positioning implies "add faultline to your existing Dolt" but doesn't address: shared vs. dedicated Dolt instance, resource contention, connection pooling, or deployment topology.
  - Suggested question: Is faultline's Dolt instance shared with Gas Town's beads Dolt, or separate? What's the deployment model — single binary, Docker compose, Kubernetes? Clarify the infrastructure reality.

- **No volume/scale expectations constrain architectural decisions.** The current architecture (single Go process, single Dolt connection, no queue, no cache) implies low volume. But "Sentry compatible" implies production use, which could mean thousands of events per second. The architecture is fundamentally different at 10 ev/s vs 1000 ev/s. Without a target, scope for performance work is unbounded.
  - Suggested question: What's the target event volume? 1 ev/s (side project), 100 ev/s (small team), 1000+ ev/s (production)? This determines whether queuing, batching, and connection pooling are in scope.

- **No data retention or cleanup policy is defined.** Error tracking systems accumulate data rapidly. Dolt's append-only commit history makes this especially acute — even deleted rows persist in history. Without retention policy, storage will grow unboundedly, and "time travel" becomes a liability rather than a feature.
  - Suggested question: What's the data retention policy? Event TTL? Issue group archival? Dolt history pruning? Is this in scope for any phase, or explicitly deferred?

- **Project management is hardcoded to environment variables.** `FAULTLINE_PROJECTS=1:default_key` is fine for a prototype but is a scope landmine. Stakeholders will inevitably ask for: project CRUD API, key rotation, multiple keys per project, secret keys (Sentry has public + secret DSN keys), per-project settings, and rate limiting. Is any of this in scope?
  - Suggested question: Is dynamic project management in scope for any phase, or is env-var config the permanent model?

### Observations

- **The phase numbering conflates priority with sequence.** P1-P5 reads as sequential phases, but some items within later phases (e.g., read-side API in P3) are arguably more valuable than some P2 items (e.g., advanced fingerprinting). The natural seams suggest: ingest (done) → read API → bead creation → dashboard → time-travel, which doesn't match the current P1-P5 ordering.

- **"While we're in there" refactor risks are moderate.** The fingerprinting system works but has known quality issues (byte-level truncation, parameterized message sensitivity). There will be pressure to "improve fingerprinting" during P2 that could expand into custom rule engines, ML-based grouping, or manual merge/split UIs — all of which are large features masquerading as improvements.

- **Multi-tenancy is not mentioned but will be asked for.** The project has "projects" (plural) with auth keys, implying multi-tenant use. But there's no organization concept, no user authentication, no RBAC, and no data isolation beyond project_id columns. If faultline is team-facing (not just personal), multi-tenancy becomes a requirement quickly.

- **The Dolt commit granularity question is a scope-defining architectural decision.** If every event triggers a Dolt commit, the "git for errors" narrative is strong but performance suffers (Dolt commits are expensive). If commits are batched/periodic, performance is better but the time-travel story weakens. This isn't just a technical detail — it determines which Dolt-native features (P5) are even possible and fundamentally shapes what "time travel for errors" means. The current implementation does standard SQL with no Dolt commits, suggesting the per-event model may already be implicitly descoped.

- **Source map support is conspicuously absent.** Any JavaScript SDK integration will immediately generate requests for source map upload and server-side stack trace deobfuscation. This is a major feature (Sentry has a dedicated service for it) that isn't mentioned anywhere. If JS SDKs are in scope, source maps will follow within days of launch.

## Confidence Assessment

**Low.** The PRD operates at pitch-deck level — it communicates vision effectively but does not function as a scope document. There are zero explicit out-of-scope statements, no compatibility matrix, no defined phase boundaries relative to current state, and the core differentiator (Gas Town integration) has no specification. The phase most likely to succeed without scope creep is P1 (largely done), but every subsequent phase has undefined boundaries. Two engineers reading this PRD would agree on direction but build materially different systems. Before implementation of P2+ can begin, the PRD needs: (1) an out-of-scope section, (2) an SDK compatibility matrix, (3) phase redefinition as deltas from current state, (4) a P4 specification, and (5) target volume/scale constraints.
