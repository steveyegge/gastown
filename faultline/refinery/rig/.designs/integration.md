# Integration Analysis

## Summary

Faultline is a 19-package Go monolith that integrates with three distinct external systems: Sentry SDKs (inbound error events), Gas Town (outbound bead filing + resolution polling), and a constellation of notification targets (Slack, GitHub, PagerDuty, webhooks). The system's internal integration is orchestrated through a single `main.go` that wires 12+ background goroutines alongside an HTTP server, all sharing a single Dolt database connection pool.

The integration architecture is fundamentally sound for its design ceiling (100 ev/s, single-server). The key integration insight is that faultline occupies a unique position as *both* a consumer and producer in the Gas Town ecosystem: it ingests SDK errors (consumer), files beads for polecats to fix (producer), and then monitors whether those fixes actually resolved the error (feedback loop). This closed-loop design is faultline's competitive advantage and its most complex integration surface.

## Analysis

### Key Considerations

- **Faultline is a bridge between two worlds.** Sentry SDKs speak HTTP envelopes; Gas Town speaks beads + git branches. The `gastown.Bridge` component translates between these two models, and its correctness is critical to the entire agentic loop.
- **All integration is runtime, not compile-time.** Integrations are configured via environment variables and database rows, not code. A faultline instance with no `FAULTLINE_RELAY_URL` silently skips relay polling. No `FAULTLINE_SLACK_WEBHOOK` means no Slack. This makes deployment flexible but debugging harder — a misconfigured integration fails silently.
- **The Dolt connection is the single point of failure.** Every package (api, ingest, gastown, dashboard, dbmon, healthmon, uptimemon) shares the same `*sql.DB` pool. If Dolt becomes slow or unavailable, all subsystems degrade simultaneously. There is no circuit breaker or fallback.
- **Background workers are fire-and-forget goroutines.** The 12+ workers (committer, bridge poller, bridge slow-burn, healthmon, uptimemon, dbmon, relay poller, snooze sweep, retention worker, etc.) are launched as `go func()` in main.go. Panic recovery exists but there's no supervisor pattern — a goroutine that exits silently stops that subsystem.

### Existing Components: What Does This Touch?

The integration surface spans every major subsystem:

| Component | Integration Role | Direction |
|-----------|-----------------|-----------|
| `ingest` | Receives Sentry SDK envelopes, parses events, stores in Dolt | Inbound |
| `gastown.Bridge` | Files beads when error thresholds are met, polls for resolution | Outbound + Feedback |
| `api` | Serves JSON for agents (polecats) and humans, CRUD for all entities | Bidirectional |
| `dashboard` | Serves HTML for human operators, same handlers as API (content negotiation) | Outbound |
| `relay` + `poller` | Store-and-forward for mobile/remote SDKs that can't reach localhost | Inbound (async) |
| `notify` | Dispatches alerts to Slack, webhooks, email on lifecycle events | Outbound |
| `integrations` | Plugin architecture for GitHub Issues, PagerDuty, Slack bot, Linear, Jira | Outbound |
| `ci` | Receives GitHub webhook events, converts CI failures to error events | Inbound |
| `dbmon` | Monitors external databases, generates synthetic error events on state changes | Internal loop |
| `healthmon` | Monitors Dolt server health + Gas Town infrastructure | Internal |
| `uptimemon` | HTTP health checks on project service URLs | Internal |
| `selfmon` | Reports faultline's own errors as Sentry events to itself | Internal loop |
| `crypto` | AES-256-GCM encryption for database monitoring connection strings | Supporting |
| `sourcemap` | JS source map resolution for readable stack traces | Supporting |
| `db` | Dolt connection, schema migrations, query builders | Foundation |

### Dependencies: What Does This Need From Others?

**Hard dependencies (system won't start without these):**
1. **Dolt server on port 3307** — All state storage. No Dolt = no faultline.
2. **Network connectivity** — HTTP server must bind to `FAULTLINE_ADDR` (default `:8080`).

**Soft dependencies (features degrade without these):**
1. **Gas Town CLI (`gt`)** — Required for bead filing. Without it, errors are tracked but no polecats are dispatched. The bridge calls `gt` as a subprocess.
2. **Public relay** — Only needed for mobile/remote SDK ingestion. Local SDKs bypass it entirely.
3. **Slack/GitHub/PagerDuty credentials** — Only needed for respective integrations. System functions without them.
4. **`FAULTLINE_DB_ENCRYPTION_KEY`** — Required for database monitoring feature. Without it, connection strings can't be encrypted and the feature is disabled.

### Dependents: What Will Depend On This?

**Upstream dependents (things that send data TO faultline):**
- Any application instrumented with a Sentry SDK (5 supported: browser, Node, iOS, Android, React Native)
- GitHub CI (via webhook) for deployment correlation
- External databases being monitored (passive — faultline connects out to them)

**Downstream dependents (things that consume data FROM faultline):**
- Gas Town polecats (consume issue context via API to diagnose and fix bugs)
- Gas Town witness (monitors bead status for resolution confirmation)
- Slack channels (receive error notifications)
- PagerDuty (receives incident alerts)
- GitHub Issues (receives auto-filed issues)
- Dashboard users (humans monitoring error state)

### Migration Path: How Do We Get From Here to There?

The system is already deployed and operational. Future integration changes should follow this migration strategy:

**Phase 1: Stabilize current integrations (P4)**
- Add health checks for all background workers (detect silent goroutine death)
- Add integration connection verification at startup (validate Dolt, relay URL, webhook URLs)
- Add structured logging for all integration lifecycle events (bead filed, bead resolved, notification sent)
- Make the `gastown.Bridge` bead creation more resilient (currently shells out to `gt` CLI — add timeout + retry)

**Phase 2: Harden integration boundaries (P5)**
- Add circuit breaker for Dolt connection (prevent cascading failure when Dolt is slow)
- Add worker supervisor pattern (restart failed goroutines with backoff)
- Move from subprocess `gt` CLI calls to direct Dolt writes for bead creation (eliminate process spawn overhead)
- Add integration health dashboard page (show status of all configured integrations)

**Phase 3: Scale integration architecture (P6+)**
- Add event queue (in-process channel or external) between ingest and processing (decouple ingestion throughput from processing speed)
- Add per-integration rate limiting (prevent notification storms from flooding Slack/PagerDuty)
- Add integration plugin SDK (allow third-party integrations without modifying faultline source)

### Backwards Compatibility: What Might Break?

**Current risks:**
1. **Dolt schema migrations are additive-only.** There's no rollback mechanism. A bad migration requires manual `ALTER TABLE` to undo. This is acceptable for single-server but blocks any future multi-instance deployment.
2. **The `ft_events` table rename** (from `events`) was a breaking change that required a coordinated migration. Future renames need the same care.
3. **API endpoints have no versioning beyond `/v1/`.** Adding breaking changes to existing endpoints requires a `/v2/` prefix or careful backward-compatible evolution.
4. **Sentry SDK compatibility is a subset.** Adding support for new Sentry features (performance traces, session replays) requires careful validation that existing silent-drop behavior doesn't break when those features are later processed.
5. **Gas Town CLI version coupling.** The `gastown.Bridge` calls `gt bead create` with a specific argument format. Changes to the `gt` CLI could break bead filing silently.

### Testing Strategy: How Do We Verify Integration?

**Current state:** Tests exist for individual packages but there's no integration test suite that exercises the full pipeline (SDK event → ingest → fingerprint → threshold → bead → resolution).

**Recommended testing layers:**

| Layer | What It Tests | How |
|-------|--------------|-----|
| **Unit** | Individual package logic (fingerprinting, rate limiting, threshold detection) | Standard Go `_test.go` files |
| **Integration** | Full pipeline: event in → bead out | Docker Compose with Dolt + faultline, send test events, verify beads created |
| **Contract** | Sentry SDK envelope format compatibility | Capture real SDK envelopes from each of 5 supported SDKs, replay through ingest handler |
| **Health** | Background worker liveness | `/health` endpoint that reports status of all workers, not just DB connectivity |
| **E2E** | Complete agentic loop | Send error → verify bead created → simulate polecat fix → verify resolution detected |

The contract tests are particularly important: Sentry SDK format evolves across SDK versions, and a format change that breaks ingestion is invisible until a real error occurs.

## Constraints Identified

1. **Single Dolt server is a hard constraint.** Faultline cannot be horizontally scaled without a shared or replicated Dolt instance. All background workers assume a single-writer model. This is acceptable for the 100 ev/s design ceiling but blocks any future scale-out.

2. **Gas Town CLI dependency creates a fragile integration point.** Bead creation via subprocess (`gt bead create`) means faultline's most important integration (the agentic loop trigger) depends on a binary being present on PATH with a compatible argument format. This is the highest-risk integration point.

3. **No integration health observability.** If the relay poller stops, or the bridge can't file beads, or Slack webhooks are failing — there's no dashboard or API endpoint to surface this. Operators discover integration failures through absence (errors not appearing, beads not filed).

4. **Background worker lifecycle is unmanaged.** Goroutines that panic or exit don't restart. The healthmon monitors Dolt health but nothing monitors the health of faultline's own workers. A crashed bridge poller means resolution detection stops silently.

5. **Notification storms are unthrottled.** A burst of 1000 errors in different issue groups could fire 1000 Slack notifications simultaneously. There's per-project rate limiting on ingestion but not on notification output.

## Open Questions

1. **Should bead creation move from `gt` CLI to direct Dolt writes?** This would eliminate the subprocess dependency but couple faultline more tightly to Gas Town's internal schema. Trade-off: reliability vs encapsulation.

2. **What's the failover strategy if Dolt becomes unavailable?** Currently: all subsystems fail. Options: write-ahead buffer for events, read-only dashboard from cache, or accept the single-point-of-failure given the self-hosted model.

3. **Should integrations be lazily loaded or eagerly validated?** Current behavior: silently skip if not configured. Alternative: explicit enable/disable per integration with startup validation of credentials.

4. **Is the 60-second Dolt commit interval still correct as the system grows?** With database monitoring, uptime monitoring, and more background writers, the volume of uncommitted writes between commits is growing. Does this affect time-travel query accuracy?

5. **How should faultline handle Gas Town version upgrades?** The `gt` CLI and beads schema may change. Is there a contract test or version compatibility check?

## Integration Points

### Connection to Data Model dimension
- Schema migration strategy directly affects integration reliability — additive-only migrations mean no rollback from a bad schema change that breaks an integration
- The `beads` table is the primary integration surface with Gas Town
- `integrations_config` table stores credentials for all external integrations

### Connection to UX dimension
- Integration health should be surfaced in the dashboard (which integrations are configured, which are healthy)
- The "Fixing — polecat dispatched" status depends on the Gas Town bridge correctly filing and tracking beads
- Notification configuration is a settings page concern — users need to know what's configured and what's not

### Connection to Security dimension
- Integration credentials (webhook secrets, API tokens, encryption keys) are the most sensitive data in the system
- The webhook verification bypass (optional HMAC) is an integration-level security concern
- Database monitoring connection strings are encrypted at rest but transmitted to the checker in plaintext in-memory

### Connection to overall system
- Faultline's core value proposition (the agentic loop) depends on three integrations working in concert: ingest (errors in) → bridge (beads out) → resolution polling (confirmation back)
- Any failure in this chain silently degrades the system from "autonomous error repair" to "a basic error tracker"
- The integration architecture should be monitored with the same rigor as the error tracking pipeline itself
