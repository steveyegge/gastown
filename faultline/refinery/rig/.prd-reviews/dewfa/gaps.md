# Missing Requirements

## Summary

The faultline PRD (README) describes a compelling vision — a self-hosted, Sentry-compatible error tracker with Dolt storage and Gas Town agentic loop integration — but it reads more as a feature pitch than a product requirements document. The five-phase roadmap names what will be built without specifying how critical cross-cutting concerns (auth, multi-tenancy, data lifecycle, operational behavior) should work. The P1 implementation reveals several areas where implicit decisions were made that the PRD never addresses, and multiple dimensions that will become production incidents if not specified before P2-P3 implementation begins.

The most critical gaps are: (1) no data retention or storage management strategy for a system that stores every raw event as JSON in Dolt, (2) no rate limiting or abuse prevention on the ingest endpoints, (3) no specification of the Dolt commit strategy (every event? batched? what triggers a Dolt commit?), and (4) no multi-tenancy isolation model beyond project_id scoping.

## Findings

### Critical Gaps / Questions

- **Data retention and storage management**: The PRD says "every error event is a git commit" but never addresses storage growth. Raw JSON events accumulate indefinitely. At even moderate volume (1K events/day), the Dolt database will grow unboundedly. There is no mention of TTL, archival, compaction, or pruning.
  - *Why this matters*: Dolt is not designed for high-write append-only workloads at scale. Without retention policies, the database will degrade over time — and "time travel" over unbounded history will become progressively slower.
  - *Suggested question*: What is the expected event volume? What is the retention policy (30 days? 90 days? forever)? Should old events be archived or purged?

- **Dolt commit strategy**: The single most Dolt-specific architectural question is completely unaddressed. The current P1 code uses standard SQL INSERT — it's unclear when/whether Dolt commits are created. Are commits per-event? Per-batch? Per-minute? The "every error event is a git commit" claim in the README may not reflect reality.
  - *Why this matters*: Dolt commit frequency directly impacts write performance, storage size, and the usefulness of time-travel features. Too many commits = slow; too few = coarse time-travel granularity.
  - *Suggested question*: What is the Dolt commit cadence? Should `DOLT_COMMIT()` be called explicitly, and if so, at what granularity?

- **Rate limiting and abuse prevention**: The ingest API is completely open once you have a valid sentry_key. There is no rate limiting per project, per IP, or globally. The `maxEnvelopeSize` of 200 MiB is the only protection.
  - *Why this matters*: A misconfigured SDK or malicious actor could flood the system with events, exhausting Dolt storage and degrading performance for all projects. This is a production incident waiting to happen.
  - *Suggested question*: What rate limits should apply? Per-project event caps? Per-IP throttling? What happens when limits are exceeded (429? silent drop?)?

- **Multi-tenancy and project isolation**: Projects are identified by integer IDs configured via environment variable. There is no concept of organizations, teams, or access control beyond "has the sentry_key." The PRD never mentions who creates projects, who can view which project's data, or how project credentials are managed.
  - *Why this matters*: If faultline is "self-hosted," is it single-tenant (one org) or multi-tenant? This fundamentally shapes the auth model, data isolation, and API design for P3's query API.
  - *Suggested question*: Is faultline single-tenant (one deployment per org) or multi-tenant? How are projects provisioned? Who has read access to error data?

- **Authentication model for the query/dashboard API (P3)**: The ingest API uses sentry_key (write-only, SDK-facing). The P3 dashboard needs a completely different auth model — user sessions, RBAC, team permissions. The PRD mentions neither.
  - *Why this matters*: The P3 query API and dashboard cannot reuse the ingest auth model. This needs to be designed before P3 begins, as it may require schema changes (users, teams, permissions tables).
  - *Suggested question*: How will humans authenticate to the dashboard? OAuth/OIDC? Local accounts? What permission model (project-level, org-level)?

- **Backwards compatibility for Sentry SDK versions**: The PRD says "Sentry SDK compatible" but doesn't specify which SDK versions or which Sentry features are in scope. Sentry's protocol has evolved significantly (envelope format, transactions, sessions, replays, profiles, check-ins, metrics).
  - *Why this matters*: The P1 implementation silently skips unknown item types (good), but there's no stated compatibility matrix. Users won't know which SDK version to use or which features work.
  - *Suggested question*: Which Sentry SDK protocol version is targeted? Which item types beyond "event" will be supported? Is transaction/performance monitoring in scope?

### Important Considerations

- **Error on the Gas Town bead creation path (P3-P4)**: The PRD says errors "automatically become beads" and polecats fix them. This is the most novel feature but has zero specification. What triggers bead creation — every new issue group? Only groups exceeding a threshold? How does faultline determine which rig to file the bead in? How is the error context (stack trace, culprit) translated into actionable bead instructions?
  - *Why this matters*: This is the entire value proposition differentiating faultline from Sentry. It deserves more than a 3-line loop diagram.

- **Resolution tracking and regression detection**: The README loop diagram mentions "marks resolved → watches for regression" but neither the PRD nor the implementation addresses how resolution status is tracked, what "regression" means (same fingerprint reappearing after close?), or how regression notifications work.
  - *Why this matters*: Without this, the "agentic loop" is incomplete — polecats can fix bugs, but faultline can't confirm the fix worked or detect regressions.

- **Concurrent access patterns**: The current `UpsertIssueGroup` uses `ON DUPLICATE KEY UPDATE event_count = event_count + 1`, which is safe for MySQL row-level locking. But concurrent envelope ingestion could produce race conditions on Dolt commits if `DOLT_COMMIT()` is called explicitly. The PRD doesn't address this.
  - *Why this matters*: Dolt's commit model is fundamentally different from MySQL's — concurrent writers may conflict at the commit level even if row-level operations succeed.

- **Data migration for schema evolution**: The P1 schema uses `CREATE TABLE IF NOT EXISTS`. What happens when P2 adds columns, changes indexes, or restructures tables? There's no migration framework or versioning strategy mentioned.
  - *Why this matters*: Dolt supports branching, but schema migrations on a live database with accumulated data need a strategy (Dolt branch per migration? ALTER TABLE? Full rebuild?).

- **Operational monitoring and alerting**: No mention of how faultline itself is monitored. Metrics (ingest latency, event count, error rate, Dolt commit latency, storage size) are critical for an error-tracking system that watches other systems.
  - *Why this matters*: "Who watches the watchmen?" If faultline goes down, errors are silently lost. There should be a strategy for self-monitoring.

- **Event deduplication across SDK retries**: The P1 implementation uses `INSERT IGNORE` on `event_id` for idempotency. But `event_id` is generated client-side (or server-side if missing). If the SDK retries with a new `event_id`, the event is double-counted in `issue_groups.event_count`.
  - *Why this matters*: `event_count` is the primary signal for issue severity. Double-counting inflates it, causing false prioritization.

- **Graceful degradation when Dolt is unavailable**: The server connects to Dolt at startup and fails hard if unavailable. There's no reconnection logic, circuit breaker, or local buffering strategy.
  - *Why this matters*: The PRD says "zero extra infrastructure" but Dolt is a hard dependency. If Dolt restarts or is briefly unreachable, all incoming events are lost (500 errors to SDKs).

- **Admin tooling for support/debugging**: No mention of how operators inspect faultline's state. How do you see which projects are configured? View event throughput? Manually merge/split issue groups? Re-fingerprint events after algorithm changes?
  - *Why this matters*: Every error tracking system needs administrative escape hatches for when the automated grouping is wrong.

- **HTTPS/TLS termination**: The server listens on plain HTTP. The PRD doesn't mention TLS. Sentry SDKs send error data that may contain PII, stack traces with source code, and authentication tokens in captured request data.
  - *Why this matters*: Error payloads frequently contain sensitive data. Transport encryption isn't optional for production use.

### Observations

- **The README is aspirational, not prescriptive**: It describes what faultline will do but not how it should behave in edge cases. This is fine for a pitch deck but insufficient as a PRD for implementation beyond P1.

- **"Zero extra infrastructure" may be overpromised**: As faultline grows through P3-P5, it will likely need background workers (for bead creation, regression detection, cleanup) that don't fit in the request-response cycle. The PRD should acknowledge this.

- **P6 (myndy_ios dogfooding) is mentioned in arch-review-input but not in the README**: This phase should be added to the roadmap if it's real, as it implies mobile-specific requirements (crash reporting, symbolication, dSYM handling) that fundamentally differ from server-side error tracking.

- **No mention of source map / symbol upload**: Sentry's ingest API supports source map uploads for JavaScript stack trace deobfuscation. If faultline targets JS SDKs, this is essential for readable stack traces — and it's a significant feature requiring file storage and processing.

- **Internationalization is likely not applicable** for a developer-facing error tracker, but **accessibility** for the P3 dashboard should be considered from the start.

- **No deployment/packaging story**: How is faultline distributed? Docker image? Go binary? Homebrew? The PRD should specify the target deployment model, as it affects configuration, TLS, and operational assumptions.

## Confidence Assessment

**Low confidence that this dimension is adequately covered.** The PRD is essentially a feature list with no cross-cutting requirements. Nearly every production concern (auth, storage, operations, data lifecycle, abuse prevention) is absent. The P1 implementation made reasonable implicit decisions, but P2+ will require explicit answers to the questions raised above. The most dangerous gaps are storage management and the Dolt commit strategy — these are architectural decisions that become very expensive to change after data accumulates.
