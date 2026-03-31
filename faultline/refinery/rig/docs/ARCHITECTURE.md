# Architecture Decisions

> This document records key architectural decisions for faultline.
> Each decision includes context, options considered, and rationale.

## ADR-1: SDK Compatibility Scope

**Decision:** Support 5 Sentry SDKs (browser, node, cocoa, android, react-native). Process `event`, `session`, and `sessions` item types. Silently drop all others.

**Context:** "Sentry SDK compatible" is unbounded — Sentry supports 30+ SDKs with dozens of item types. We needed an explicit boundary.

**Options considered:**
- (a) Full Sentry compatibility — years of work, not feasible
- (b) Error events only — too narrow, misses session tracking
- (c) Events + sessions for 5 target SDKs — covers mobile + web MVP

**Rationale:** All 5 target SDKs send `event` and `session` by default. Silent drop of other items causes zero client-side errors (SDKs fire-and-forget). This covers the core error tracking use case without scope explosion.

**Future:** Source maps/symbolication added in P6 for readable JS/mobile stack traces.

See: [docs/SDK-COMPATIBILITY.md](SDK-COMPATIBILITY.md)

## ADR-2: Dolt Commit Strategy

**Decision:** Batch commits every 60 seconds.

**Context:** "Every error event is a git commit" was aspirational. Per-event Dolt commits cost ~5-10ms each, making them a bottleneck at any real event volume.

**Options considered:**
- (a) Per-event commits — true time-travel, ~50 ev/s ceiling
- (b) Periodic batch commits (60s) — practical, 60s time-travel granularity
- (c) Event-driven commits (on new issue group) — elegant but complex
- (d) No commits, revisit in P5 — simplest but defers Dolt's value

**Rationale:** Batch commits match gastown's proven pattern. 60s granularity is sufficient for "what did the error landscape look like yesterday?" queries. At 100 ev/s target, this means ~6000 events per commit — well within Dolt's capability.

**Implementation:** Background goroutine calls `CALL dolt_add('-A')` then `CALL dolt_commit('-m', 'ingest: {event_count} events')` every 60s. Skip if no writes since last commit.

## ADR-3: Gas Town Integration Contract

**Decision:** Bead creation triggered by 3+ events in 5 minutes for error/fatal severity. Resolution via bead status polling + quiet period confirmation.

### Bead Creation Trigger
- New issue group appears with `level` = `error` or `fatal`
- 3+ events accumulate within a 5-minute window
- One bead per issue group (deduplication by group ID)
- Configurable per-project thresholds in future

### Bead Content
- Exception type + message (title)
- Culprit (top stack frame: module.function)
- Event count + first/last seen timestamps
- Sample raw event JSON (most recent)
- Link to target rig (from project config)

### Project → Rig Mapping
- New config field: `FAULTLINE_PROJECTS=project_id:key:rig_name`
- Example: `1:abc123:faultline,2:def456:myndy_api`
- Bead filed in the target rig's beads directory

### Resolution Detection
- Faultline polls bead status periodically (every 60s)
- When bead status = `closed`: start quiet period timer
- No new events for configured period (default 10 min) → mark resolved
- New events during quiet period → cancel resolution, bead stays open

### Failure Path
- Polecat calls `gt done --status ESCALATED` if it can't fix
- Faultline marks issue group as "needs_human"
- Mail sent to crew or overseer for manual triage

## ADR-4: Regression Definition

**Decision:** Same fingerprint reappearing within 24 hours of resolution triggers regression (bead reopen).

**Context:** Regression detection ranges from trivial (same hash = reopen) to complex (statistical detection with configurable windows). Sentry considers this one of their most complex features.

**Options considered:**
- (a) Simple reopen — any reappearance after close
- (b) 24h window reopen — regression only if within 24h of resolution
- (c) Full state machine — resolved → regressed → archived with configurable windows

**Rationale:** (b) covers the primary use case ("deploy broke it again") without overengineering. After 24h, a reappearing fingerprint is more likely a new context (different deploy, different trigger) than a regression.

**Future evolution:** Full state machine (c) documented for P6 if needed. Would add: configurable regression window per project, auto-archive after N days quiet, regression count tracking.

## ADR-5: Target Event Volume

**Decision:** 100 events/second sustained as the design ceiling.

**Context:** Load testing showed Dolt handles burst well (1500 ev/s at c=50) but sustained load needs connection pooling and write batching.

**Options considered:**
- (a) 1-10 ev/s — personal projects
- (b) 10-100 ev/s — small teams
- (c) 100-500 ev/s — mid-size production
- (d) 1000+ ev/s — fleet scale

**Rationale:** Faultline's Gas Town agentic loop is most valuable for small-to-mid teams where polecats can actually fix bugs autonomously. At fleet scale (1000+ ev/s), errors are triaged by oncall humans, not agents — those teams run real Sentry. 100 ev/s sustained covers any team small enough to benefit from the agentic loop.

**Implications:**
- No write-ahead buffer needed (direct Dolt writes suffice)
- Connection pool: 25 max open, 10 idle (already configured)
- Batch Dolt commits every 60s (~6000 events per commit at ceiling)
- Rate limiting returns 429 when exceeded

## ADR-6: License

**Decision:** MPL-2.0 (Mozilla Public License 2.0).

**Context:** Needed a license that allows free use but requires contributing back modifications to faultline source files.

**Options considered:**
- MIT/Apache-2.0 — no copyleft, changes can stay proprietary
- MPL-2.0 — file-level copyleft, modifications must be shared
- AGPL-3.0 — program-level copyleft, even SaaS must share (scares enterprise)

**Rationale:** MPL-2.0 is the sweet spot. Used by HashiCorp (Terraform, Vault), Mozilla Firefox. Anyone can use faultline commercially. If they modify a faultline source file, they share that file. They can combine faultline with proprietary code without "infection." Stronger than MIT, less aggressive than AGPL.
