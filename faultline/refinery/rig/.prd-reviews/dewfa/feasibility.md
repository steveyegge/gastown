# Technical Feasibility

## Summary

Faultline's core premise — a self-hosted Sentry-compatible error tracker backed by Dolt — is technically sound and partially proven. P1 (Sentry envelope/store ingest) is already implemented with clean architecture, correct protocol handling, and 15 passing unit tests. The hardest remaining problems are: (1) making Dolt perform acceptably as a high-write event store without the column-store engines Sentry uses, (2) reliably bridging the Sentry event model into Gas Town's bead system without creating bead storms, and (3) handling the Sentry SDK protocol's full surface area (which is vast and poorly documented).

The project is buildable through P3. P4 (agentic loop) and P5 (Dolt-native features) carry meaningful technical risk around Dolt write throughput and branch management at scale. None of the risks are showstoppers, but several could double implementation effort if discovered mid-build.

## Findings

### Critical Gaps / Questions

- **Dolt write throughput under sustained event load.** Sentry SDKs batch events and can burst hundreds per second during an incident. Dolt commits are expensive (each is a structural merge of a Prolly tree). The current schema does `INSERT IGNORE` per event with no batching or write coalescing. At what sustained event rate does Dolt become the bottleneck?
  - Why this matters: If Dolt can't keep up, events queue unboundedly in memory (no backpressure exists), and the server OOMs or drops events silently.
  - Suggested question: What's the target event throughput? 1/s (personal project) vs 100/s (production service) vs 10k/s (fleet) requires fundamentally different storage strategies.

- **Bead creation storm prevention (P3/P4).** The PRD says new issue groups become beads and trigger polecat work. During a deploy gone wrong, a single root cause can produce dozens of distinct fingerprints (different stack depths, different goroutines, race-dependent frames). Without deduplication/cooldown at the bead-creation boundary, one bad deploy could spawn 50+ polecats working on the same underlying bug.
  - Why this matters: Gas Town's Dolt server is already identified as fragile. A bead storm taxes both the beads Dolt instance and the witness/polecat scheduler.
  - Suggested question: What's the deduplication strategy at the error→bead boundary? Time-based cooldown? Similarity threshold across fingerprints? Manual triage gate?

- **Sentry protocol surface area.** The Sentry SDK protocol is large, versioned, and partially undocumented. P1 handles envelopes and store endpoints, but real SDKs also send: sessions, transactions (performance monitoring), attachments, user feedback, check-ins (cron monitoring), profiles, replays, and metrics. Each item type has its own schema. The current code skips unknown types (good), but clients may behave unexpectedly if expected response headers or DSN negotiation endpoints are missing.
  - Why this matters: "Sentry SDK compatible" implies drop-in replacement. Real-world SDKs do capability negotiation, rate-limit header parsing, and retry with backoff based on `Retry-After` and `X-Sentry-Rate-Limits` headers. Missing these causes SDK-side misbehavior (infinite retries, dropped events).
  - Suggested question: Which Sentry SDKs are targeted? (sentry-python, sentry-javascript, sentry-cocoa each have different protocol expectations.) Is "compatible" scoped to error events only, or the full observability suite?

### Important Considerations

- **No transaction safety in the ingest path.** The handler does `UpsertIssueGroup` then `InsertEvent` as separate operations. If the process crashes between them, the issue group's `event_count` is incremented but the event doesn't exist. This is tolerable for analytics but becomes a data integrity issue if beads are created based on event counts or group state.
  - Mitigation: Wrap both operations in a single Dolt transaction. Dolt supports standard MySQL transactions.

- **No connection pooling or liveness checks.** The DB layer opens one connection via `sql.Open` with no pool configuration. Under load, Go's `database/sql` will create connections up to `MaxOpenConns` (default unlimited), potentially overwhelming Dolt's single-threaded query processor.
  - Mitigation: Set `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`. Add a `/health` check that pings Dolt (current health endpoint doesn't verify DB).

- **Dolt `dolt_commit` integration missing.** Dolt's unique value proposition (time-travel, branching, diffing) requires explicit `CALL dolt_commit()` to create versioned snapshots. The current code does standard SQL inserts which land in the working set but aren't committed to Dolt's version history. Without this, P5 (time-travel, fingerprint branching) has no foundation.
  - Why this matters: This is architectural — retrofitting Dolt commits into a high-throughput write path requires careful batching (commit-per-event is prohibitively expensive, commit-per-batch needs a flush strategy).
  - Suggested question: What's the Dolt commit cadence? Per-event (expensive), periodic (lossy window), or on-demand (manual)?

- **No rate limiting or backpressure.** Sentry's ingest API returns `429` with `Retry-After` headers when overloaded. SDKs respect this to shed load gracefully. Faultline has no rate limiting, meaning a burst of events will be accepted until the system falls over.
  - Mitigation: Implement per-project rate limiting with `429` responses and `Retry-After` headers. This is essential for SDK compatibility, not just operational safety.

- **1 MiB per-item limit may be too small.** Sentry's default is 1 MiB for errors but some SDKs send larger payloads with breadcrumbs, context, and attached stack traces (especially native/mobile SDKs with full thread dumps). The 200 MiB envelope limit is generous but the 1 MiB item limit could silently truncate real-world events.
  - Suggested question: Is the 1 MiB limit tested against real SDK output from target platforms?

### Observations

- **Fingerprinting is solid.** The SHA256 priority chain (SDK → exception+frames → message → unknown) matches Sentry's grouping logic closely. The title/culprit extraction is correct. This is a hard problem done well.

- **Auth model is intentionally simple.** Project key pairs via environment variable work for single-tenant, but won't scale to multi-tenant. This is fine for the stated scope (self-hosted) but worth noting for future.

- **Dependency footprint is minimal.** Three production dependencies (mysql driver, uuid, edwards25519) is excellent. No framework overhead, no ORM, no middleware chain. This keeps the attack surface small and build times fast.

- **No observability of the observer.** Faultline is an error tracker with no error tracking of itself. It uses `slog` structured logging, which is appropriate for P1, but by P3 it should probably report its own errors to itself (or a separate instance) to dogfood the system.

- **The Go + Dolt + Gas Town stack is novel.** There's no prior art for this exact combination. Dolt-as-event-store is unproven at scale. This is the biggest technical bet in the project — it either validates Dolt for write-heavy OLTP workloads or discovers fundamental limitations.

## Confidence Assessment

**Medium-High.** The P1 implementation demonstrates the core ingest path works. P2 (schema refinement, better grouping) and P3 (query API) are straightforward extensions of what exists. The hard unknowns are concentrated in P4 (agentic loop — bead storm prevention, bidirectional state sync between faultline and Gas Town) and P5 (Dolt-native features — write throughput under `dolt_commit`, branch management). The biggest risk that could double effort is discovering Dolt can't sustain the write throughput needed for real-world error volumes, requiring either aggressive batching/buffering or a hybrid storage approach.
