# Requirements Completeness

## Summary

The faultline PRD has matured significantly since the initial review. The README now functions as a proper specification for P1-P2 scope, with an SDK compatibility matrix (docs/SDK-COMPATIBILITY.md), architecture decision records (docs/ARCHITECTURE.md), a design brief with dashboard screens (docs/DESIGN-BRIEF.md), and explicit out-of-scope declarations. The seven critical questions raised in the previous review (dewfa) have all been answered: SDK scope is bounded to 5 SDKs, Dolt commits are batched every 60s, the Gas Town integration contract is specified in ADR-3, regression means 24h reopen window, target volume is 100 ev/s, and phase boundaries are clearer.

However, the PRD still lacks testable acceptance criteria for individual features, has no formal definition of "done" per phase, and several important subsystems (authentication/RBAC, data retention enforcement, self-monitoring) are described at design-intent level rather than specification level. A QA engineer could write integration tests for the ingest path and Gas Town bridge from this PRD, but could not write acceptance tests for the dashboard, account model, or operational behaviors without further clarification.

## Findings

### Critical Gaps / Questions

- **No per-phase acceptance criteria or definition of done.** The PRD describes features per phase but never states the exit criteria. For example: when is the dashboard "done"? When all 4 screens from the design brief render? When keyboard navigation works? When RBAC is enforced? The architecture decisions doc (ADR-1 through ADR-6) records *what* was decided but not *what must be true* for the decision to be validated.
  - Why this matters: Without acceptance criteria, scope creep is invisible. An engineer can keep adding dashboard features indefinitely without a clear "ship it" signal.
  - Suggested clarifying question: "For each phase, list 3-5 concrete acceptance tests that, if passing, mean the phase is complete."

- **Authentication and RBAC are described but not specified.** The design brief defines 5 roles (Owner, Admin, Member, Viewer, Agent) with a permissions table, but the actual permission checks are not mapped to API endpoints. Which endpoints require which role? Can a Viewer access `/api/{project_id}/issues/{issue_id}/context/`? Can an Agent resolve issues? The current implementation uses Bearer tokens with no role scoping.
  - Why this matters: Auth is a cross-cutting concern that touches every endpoint. Retrofitting RBAC onto an existing API is error-prone. Without a permissions matrix (role x endpoint), implementation will be inconsistent.
  - Suggested clarifying question: "Provide a role x endpoint matrix showing which roles can access which API endpoints."

- **Data retention is configured but enforcement is unspecified.** `FAULTLINE_RETENTION_DAYS` defaults to 90 but the PRD doesn't describe the enforcement mechanism. Is it a cron job? A background goroutine? Does it delete from Dolt's working set, or does it also prune Dolt commit history? Dolt's append-only commit history means deleted rows still exist in historical commits — does retention mean "not queryable" or "physically removed"?
  - Why this matters: Dolt storage grows monotonically unless history is explicitly pruned with `dolt gc`. A 90-day retention policy on the working set without history pruning will eventually exhaust disk.
  - Suggested clarifying question: "Does retention mean logical deletion (removed from current tables) or physical deletion (pruned from Dolt history via garbage collection)?"

- **Self-monitoring is mentioned (`FAULTLINE_SELFMON_KEY`) but not defined.** What does faultline monitor about itself? Does it send its own panics as events? Does it track ingest latency, dropped events, DB errors? The `/health` endpoint behavior is not specified beyond "returns 200" — does it check DB connectivity, disk space, goroutine count?
  - Why this matters: An error tracker that can't report its own failures is an operational risk. The config variable exists but the behavior is undefined.
  - Suggested clarifying question: "What self-monitoring events does faultline emit? What does the health endpoint actually check?"

### Important Considerations

- **Bead storm prevention is partially specified but untestable.** ADR-3 defines "3+ events in 5 minutes" as the bead creation trigger and "one bead per issue group" as deduplication. But the PRD doesn't specify behavior when multiple distinct fingerprints share a root cause (e.g., a bad deploy producing 20 different exception types). The `FAULTLINE_RATE_LIMIT` (100 ev/s) limits ingest but not bead creation rate. A global bead creation rate limit or cooldown period is not specified.
  - Suggested addition: Define a maximum bead creation rate per project per time window (e.g., max 5 beads per project per 10 minutes).

- **Relay security model is implicit.** The relay accepts envelopes from the internet and stores them in SQLite. The local faultline instance polls and acknowledges. But: who can write to the relay? Is the relay authenticated? Can a malicious actor flood the relay with fake envelopes? The relay DSN includes a public key, but it's unclear if the relay validates it or just stores everything.
  - Suggested addition: Specify relay authentication and rate limiting.

- **Webhook security is specified (HMAC-SHA256) but webhook failure behavior is not.** What happens when the GitHub webhook endpoint receives a malformed payload? When the HMAC doesn't match? When the resolve webhook fires but the referenced bead doesn't exist? The PRD specifies the auth mechanism but not the error responses.
  - Suggested addition: Define expected HTTP responses for webhook error cases.

- **Source map upload endpoint exists in the API reference but is listed as "out of scope" in SDK-COMPATIBILITY.md.** The README lists `POST /api/{project_id}/sourcemaps/` as a supported endpoint, but SDK-COMPATIBILITY.md explicitly says "Server-side symbolication / source map processing" is not planned. This is a contradiction — the endpoint exists but the feature doesn't.
  - Suggested resolution: Either remove the source map endpoints from the API reference or clarify that upload is supported but server-side processing is deferred.

- **Dashboard interaction model is described in the design brief but not in the README.** The design brief defines keyboard shortcuts (j/k/enter/r/i), filter syntax (`level:fatal project:myapp`), and three screen types. None of this appears in the README's dashboard section. If the README is the PRD, these requirements are missing from the spec. If the design brief is authoritative, the README should reference it.
  - Suggested resolution: The README dashboard section should explicitly defer to the design brief or incorporate its key requirements.

- **No migration strategy between phases.** As the schema evolves (new tables for accounts, organizations, roles, sessions), how are existing deployments upgraded? Is there an auto-migration on startup? A CLI command? Dolt branch-based migration? The PRD is silent on this.

### Observations

- **The ADR document is excellent.** ADR-1 through ADR-6 address the exact questions raised in the previous review. Options considered, rationale, and future evolution are all documented. This is the strongest part of the specification.

- **Configuration is well-specified.** The environment variables table covers rate limiting, retention, PII scrubbing, relay, self-monitoring, and health checks. Reasonable defaults are provided. This is buildable-from.

- **The Gas Town integration contract (ADR-3) is the most improved section.** Bead creation triggers, content schema, resolution detection, and failure path are all defined. The only gap is bead storm prevention at scale.

- **The "architecture decisions" table in the README is a good summary.** It links decisions to rationale and provides quick answers (5 SDKs, batch commits, 3+ events trigger, 24h regression, 100 ev/s, MPL-2.0).

- **Phase boundaries are still somewhat ambiguous.** The README no longer uses numbered phases (P1-P5) in the main text, but the design brief references P4/P5/P6. The mapping between features and phases should be consolidated in one place.

- **The existing `dewfa/requirements.md` review flagged 5 critical gaps. All 5 have been addressed** (SDK scope, Gas Town contract, performance targets, error handling via rate limiting, and phase boundaries). The PRD has been responsive to review feedback.

## Confidence Assessment

**Medium.** The PRD has moved from pitch-deck to working specification for the core ingest and Gas Town bridge. A developer could build the ingest pipeline, fingerprinting, bead creation, and resolution detection from this spec. However, the dashboard, account model, RBAC, data retention enforcement, and operational behaviors remain at design-intent level — enough to start implementation but not enough to write acceptance tests. The critical gap is not "what should we build?" (that's answered) but "how do we know it's done?" (still missing per-phase acceptance criteria).
