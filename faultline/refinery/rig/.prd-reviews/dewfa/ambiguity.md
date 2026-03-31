# Ambiguity Analysis

## Summary

The faultline PRD (README.md) presents a compelling vision — a self-hosted Sentry replacement built on Dolt with Gas Town agentic integration — but operates at a pitch-deck level of specificity. Most of the core claims are open to multiple reasonable interpretations, and the phase breakdown conflates "what ships when" with "what it does." Two engineers reading this PRD would agree on the general direction but diverge sharply on scope, compatibility boundaries, and integration semantics. The automated error-to-fix loop (the core differentiator) is described narratively but has no defined triggering conditions, failure modes, or boundary contracts.

## Findings

### Critical Gaps / Questions

- **"Sentry SDK compatible — drop-in replacement for any Sentry SDK"**
  The word "any" is doing enormous work. Sentry supports SDKs for 30+ platforms (Python, JavaScript, Go, Java, Ruby, .NET, Rust, React Native, Flutter, etc.), each with different envelope schemas, attachment types, session tracking, performance tracing, release health, and feature flags. The current implementation handles `event`, `transaction`, and `error` item types and skips everything else. "Drop-in replacement" implies zero client-side changes, but the DSN format, project key provisioning, and feature negotiation are all unspecified.
  - **Why this matters:** An engineer might implement full session replay ingest because "drop-in" implies it; another might ship events-only and call it done. This is the #1 scope risk.
  - **Suggested question:** Which Sentry SDK features are in scope? Define an explicit compatibility matrix (e.g., "Python SDK v2.x error events only, no performance tracing, no session replay").

- **"every error event is a git commit"**
  Does each ingested event trigger a Dolt commit? The current implementation uses standard SQL INSERT/UPSERT with no `CALL dolt_commit()`. If the intent is one-commit-per-event, this has massive performance implications (Dolt commits are expensive). If it means "events are stored in a Dolt database which can be committed," that's a much weaker claim and the time-travel story is about manual/batch commits, not per-event versioning.
  - **Why this matters:** This determines the entire storage architecture. Per-event commits give true time-travel per error but will not scale. Batch commits give performance but weaken the "git for errors" narrative.
  - **Suggested question:** What is the Dolt commit granularity? Per-event, per-batch, periodic, or manual? What exactly is time-travelable?

- **"errors automatically become beads; polecats fix them"**
  This is the core differentiator but has no specification:
  - What triggers bead creation? Every new issue group? Only groups above a threshold? Only in certain projects?
  - What information goes into the bead? Raw stack trace? Grouped summary? Link to faultline UI?
  - What does "polecats fix them" mean concretely? The polecat receives a bead with... what instructions? How does it know which repo to look at?
  - What does "closed when PR merges" mean? How does faultline know a PR fixed a specific error group?
  - **Why this matters:** This is the entire P4 phase and the project's raison d'etre. Without this specified, P1-P3 are just "worse Sentry."
  - **Suggested question:** Define the bead creation trigger, bead content schema, polecat assignment strategy, and resolution detection mechanism.

- **"watches for regression"**
  Completely undefined. Does this mean: same fingerprint reappears after resolution? Same exception type from the same code path? Within what time window? What action is taken — reopen bead, create new bead, alert, auto-assign polecat?
  - **Why this matters:** Regression detection is a feature that Sentry itself considers complex (resolved vs. regressed vs. archived states). Without definition, this is either trivial (same hash reappears = reopen) or a multi-sprint feature (statistical regression detection).
  - **Suggested question:** Define "regression" precisely. Is it fingerprint re-occurrence after close? What's the resolution state machine?

- **Phase boundaries are undefined**
  "P2: Dolt schema + issue grouping + fingerprinting" — but P1 already implements schema (events + issue_groups tables), fingerprinting (SHA256-based), and issue grouping (group_hash upsert). What is P2 actually adding beyond what P1 shipped? Similarly, P3 says "Query REST API" but the ingest endpoints are already REST. Is P3 about read endpoints (list issues, get event details, search)?
  - **Why this matters:** Engineers can't estimate work or plan sprints when phase contents overlap with already-shipped code.
  - **Suggested question:** Redefine phases as delta from current state. What does each phase ADD that doesn't exist yet?

### Important Considerations

- **"Zero extra infrastructure — runs on your existing Dolt server"**
  This assumes the user already runs a Dolt server. For anyone who doesn't, faultline IS extra infrastructure (Go server + Dolt server). The claim also implies faultline shares a Dolt instance with other databases, but there's no discussion of resource contention, connection pooling, or isolation. The Gas Town CLAUDE.md already documents Dolt fragility concerns.
  - **Suggested question:** Is faultline's Dolt instance shared with Gas Town's Dolt, or separate? What's the deployment topology?

- **Project authentication model is underspecified**
  Projects are configured via `FAULTLINE_PROJECTS=1:default_key` environment variable. There's no mention of: project CRUD API, key rotation, multiple keys per project, secret keys (Sentry has public + secret), rate limiting per project, or project-level configuration (allowed origins, data scrubbing rules).
  - **Suggested question:** Is static env-var config the permanent model, or is there a project management API in a later phase?

- **"should" vs "must" — the entire PRD uses neither**
  The README uses declarative statements ("Sentry SDK compatible", "every error event is a git commit") that read as absolute claims but may be aspirational. There is no RFC-2119-style language distinguishing requirements from goals. Every bullet is stated with equal weight.
  - **Suggested question:** Which claims are hard requirements vs. north-star goals? Mark each phase's deliverables as MUST/SHOULD/COULD.

- **Dashboard scope (P5 → actually P3) is "React/Vite (dashboard)"**
  The Stack section says "React/Vite (dashboard — P3)" but gives zero indication of what the dashboard shows. Issue list? Event detail? Stack trace viewer? Graphs? Search? Filters? Real-time streaming? A "dashboard" could be a single status page or a full Sentry-like UI. This is potentially the largest phase by engineering effort.
  - **Suggested question:** What are the dashboard's screens/views? What data does it expose? Is it view-only or does it support actions (resolve, ignore, assign)?

- **Message fingerprinting is fragile for parameterized messages**
  The current fingerprint fallback hashes the raw `message` field. Messages like "Failed to connect to db after 3 retries" and "Failed to connect to db after 5 retries" produce different fingerprints, creating separate issue groups for the same logical error. Sentry handles this via parameterized messages (`logentry.message` with `%s` placeholders). The PRD doesn't address fingerprint quality or tuning.
  - **Suggested question:** Is fingerprint quality in scope? Should message-based fingerprinting strip numbers/UUIDs? Is custom fingerprint rule configuration planned?

### Observations

- **The loop diagram implies synchronous flow but the system is async.** The arrow from "polecat fixes → gt done → refinery merges → faultline marks resolved" implies a callback from the merge system to faultline. No such integration is described or implemented. This is a cross-system contract that needs explicit definition.

- **No mention of data retention or cleanup.** Error tracking systems accumulate data fast. There's no discussion of event TTL, storage limits, archival, or pruning. Dolt's append-only commit history makes this especially important.

- **No multi-tenancy or access control discussion.** Projects exist but there's no concept of organizations, teams, or user authentication for the dashboard. If this is single-tenant only, that should be stated.

- **"Dolt-native features (time travel, fingerprint branching)" in P5** — "fingerprint branching" is not a well-known concept. Does this mean creating a Dolt branch per fingerprint? Per issue group? What's the use case? This sounds novel but is completely undefined.

- **No error/event volume expectations.** The architecture (single Go process, single Dolt connection, no queue) implies low volume, but the Sentry compatibility claim implies production use. What's the target scale — 10 events/sec? 1000? This determines whether the current architecture is viable or needs queuing/batching.

- **`truncate()` operates on bytes, not runes.** Minor implementation note: `truncate(s, 200)` in fingerprint.go can split a multi-byte UTF-8 character. Not an ambiguity in the PRD per se, but evidence of implicit ASCII-only assumptions.

## Confidence Assessment

**Low-Medium.** The PRD is extremely concise — more of a project pitch than a specification. The core value proposition (Sentry-compatible ingest + Dolt storage + Gas Town auto-fix loop) is clear directionally, but nearly every functional boundary is undefined. The highest-risk ambiguities are: (1) what "Sentry SDK compatible" actually means in terms of supported features, (2) the Dolt commit granularity, and (3) the entire Gas Town integration contract (P4). Without resolving these, different implementers would build materially different systems.
