# Requirements Completeness

## Summary

The faultline PRD (README.md) provides a compelling vision — a self-hosted Sentry-compatible error tracker backed by Dolt with Gas Town agentic integration — but it is a *pitch deck*, not a requirements specification. It defines the loop and the phases at a very high level but provides almost no success criteria, acceptance conditions, performance targets, failure mode handling, or testable requirements. The actual implementation (Phase 1) went well beyond what the PRD specifies, suggesting the engineering team is filling in requirements implicitly rather than working from a spec.

The biggest risk: Phases 3-5 involve deep integration with Gas Town (bead creation, polecat triggering, resolution tracking, regression detection) and Dolt-native features (time travel, fingerprint branching). These are architecturally complex and the PRD provides zero guidance on how they should work, what success looks like, or how failures should be handled. An engineer starting Phase 3 today would have to invent the requirements.

## Findings

### Critical Gaps / Questions

- **No success criteria for any phase.** The PRD lists P1-P5 as bullet points but never defines what "done" means for any of them. When is P1 complete? When the first envelope is accepted? When all Sentry SDK event types are supported? When there's 99.9% compatibility with the Sentry envelope spec?
  - *Why this matters:* Without success criteria, there's no way to verify completion or scope creep. The current P1 implementation makes choices (e.g., skipping attachment and session item types) that may or may not be acceptable — we can't tell from the PRD.
  - *Suggested question:* "For each phase, what is the minimum set of functionality that constitutes 'done'? What Sentry SDK features are explicitly out of scope?"

- **No acceptance conditions for Sentry compatibility.** "Sentry SDK compatible" and "drop-in replacement" are claims without definition. Which SDKs? Which versions? Which event types (errors, transactions, sessions, replays, profiles, check-ins)?
  - *Why this matters:* The current implementation handles `event`, `transaction`, and `error` item types but skips `attachment`, `session`, `user_report`, and many others. Is that a bug or intended?
  - *Suggested question:* "Which Sentry SDKs and event types must be supported? Is there a compatibility matrix?"

- **No definition of the Gas Town integration contract (Phases 3-4).** The loop diagram shows `creates bead → witness detects → slungs polecat → polecat fixes → gt done → refinery merges → faultline marks resolved → watches for regression`. Every arrow in that chain is an unspecified interface.
  - *Why this matters:* This is the core differentiator of faultline. Without specifying the bead creation trigger, the polecat assignment mechanism, the resolution detection signal, and the regression watch algorithm, implementation will be ad hoc.
  - *Suggested question:* "What is the exact contract between faultline and Gas Town? When does a new issue_group become a bead? What constitutes 'regression'?"

- **No performance or scale requirements.** How many events/second should faultline handle? What's the expected event size? How many projects? How long is data retained?
  - *Why this matters:* The current implementation reads entire envelopes into memory (up to 200 MiB), does synchronous DB writes per event, and has no batching. These are fine for small scale but will break under load. Without targets, there's no way to know when optimization is needed.
  - *Suggested question:* "What is the expected event volume? 1/s? 100/s? 10,000/s? How many projects and how much history should be retained?"

- **No error handling or failure mode requirements.** What happens when Dolt is down? When an event is malformed? When the disk is full? When a fingerprint collision occurs?
  - *Why this matters:* The implementation makes reasonable choices (returns 500 on DB errors, skips malformed items, uses INSERT IGNORE for idempotency), but these are engineering judgment calls, not specified behavior. A QA engineer cannot write failure-mode tests from this PRD.
  - *Suggested question:* "What is the expected behavior when the database is unavailable? Should events be buffered, dropped, or rejected with a specific status code?"

### Important Considerations

- **No monitoring, alerting, or observability requirements.** The PRD mentions none. The implementation has structured logging (slog JSON) and a `/health` endpoint that returns HTTP 200 without checking DB connectivity. For an error-tracking system, the irony of having no error tracking on itself is notable.
  - *Suggested addition:* Define what metrics faultline should expose (events ingested, events dropped, DB latency, queue depth) and what alerts should fire.

- **No security requirements beyond project auth.** The current auth is a shared API key per project (public key only, no secret). There's no mention of TLS, rate limiting, input sanitization beyond size limits, or access control for the future REST API.
  - *Suggested addition:* Define the threat model. Who can send events? Who can read them? Is the REST API authenticated?

- **No data model specification.** The schema was designed during implementation (two tables: `events` and `issue_groups`). Phases 2-5 will need additional tables (projects, users, releases, environments, tags, etc.) but the PRD provides no data model guidance.
  - *Suggested addition:* At minimum, define what entities exist and their relationships.

- **"Zero extra infrastructure" claim is untested.** The PRD claims faultline runs on your existing Dolt server, but the implementation requires a dedicated `faultline` database on that server. The claim should be validated: what happens when faultline shares Dolt with production beads databases under load?

- **No rollback or migration requirements.** As the schema evolves through phases, how are migrations handled? Dolt supports branching — is that the migration strategy? What about breaking schema changes?

### Observations

- **Implementation exceeds PRD.** The codebase is more thoughtful than the PRD warrants — idempotent inserts, gzip support, multi-format timestamp parsing, graceful shutdown, envelope size limits. This suggests capable engineers, but it also means the PRD is not the source of truth.

- **Phase boundaries are ambiguous.** P2 is "Dolt schema + issue grouping + fingerprinting" but the current P1 implementation already includes schema, issue grouping, and fingerprinting. The phases don't match what was built.

- **The Dolt-native features (P5) are the most speculative.** "Time travel" and "fingerprint branching" are interesting ideas but completely undefined. These could be trivial (add `AS OF` to queries) or massive (full branching workflow for experimental grouping strategies).

- **No mention of the dashboard (P3) beyond "React/Vite".** What does the dashboard show? What actions can users take? Is it read-only? This is an entire product surface with zero requirements.

## Confidence Assessment

**Low.** The PRD is a project pitch, not a requirements document. It successfully communicates the vision and value proposition but is not buildable-from. A QA engineer could not write acceptance tests from this document. An engineer starting Phase 3 would need to invent requirements wholesale. The gap between what's written and what's needed grows significantly in later phases where Gas Town integration and Dolt-native features require precise interface specifications.
