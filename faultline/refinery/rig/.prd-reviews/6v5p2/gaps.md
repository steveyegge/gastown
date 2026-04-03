# Missing Requirements

## Summary

The PRD has matured significantly since the dewfa review round. Critical questions about SDK scope, Dolt commit strategy, Gas Town contracts, regression semantics, and target volume are now answered in README.md and ARCHITECTURE.md. However, several requirement categories remain completely absent: there is no specification for how faultline upgrades itself (schema migrations on a running system), no backup/disaster recovery story for Dolt data, no specification for the relay's security model, and no requirements around what happens when the Gas Town target rig is unreachable. The PRD also describes features (organizations, teams, RBAC roles) in DESIGN-BRIEF.md that have no corresponding specification in the main PRD, creating a shadow scope problem.

## Findings

### Critical Gaps

**G1: No schema migration strategy for upgrades**
- The codebase uses `CREATE TABLE IF NOT EXISTS` for every table (16+ migrate functions chained in `dolt.go:60-130`). This works for fresh installs but has no mechanism for ALTER TABLE migrations on existing data. Adding a column, changing a type, or restructuring a table on a running instance is unspecified.
- Why this matters: The first time a faultline upgrade needs to modify an existing column or add a NOT NULL column with a default, the current approach breaks. Every production database tool needs a migration strategy.
- Suggested clarifying question: What is the upgrade path for a running faultline instance? Will you use numbered migrations, Dolt branch-based schema evolution, or manual ALTER TABLE?

**G2: No backup, restore, or disaster recovery specification**
- Dolt provides `dolt clone`, `dolt backup`, and remote push capabilities, but the PRD mentions none of them. There is no guidance on how to back up a faultline instance, recover from data loss, or replicate data to a secondary.
- Why this matters: Faultline stores error history that polecats depend on for diagnosis. Loss of this data means loss of the agentic loop's memory. For any team relying on faultline in production (the stated use case), backup is table stakes.
- Suggested clarifying question: Should faultline push to a Dolt remote on a schedule? Is there a `faultline backup` command? What's the recovery playbook if the Dolt data directory is corrupted?

**G3: No specification for relay authentication and security**
- The relay on fly.dev accepts envelopes from any SDK with a valid DSN key and stores them for up to 7 days. But the relay-to-local polling path (`GET /relay/poll`, `POST /relay/ack`) has no documented authentication. There's no specification for: who can poll, how the relay validates the polling client, whether envelopes are encrypted at rest, or what happens if the relay is compromised.
- Why this matters: The relay is a public internet endpoint storing raw error payloads (which may contain PII despite scrubbing). An unauthenticated poll endpoint means anyone who discovers it can read all stored envelopes. The PRD mentions `FAULTLINE_SCRUB_PII=true` but scrubbing happens server-side after polling, not at the relay.
- Suggested clarifying question: How does the relay authenticate poll requests? Is there a shared secret? Are envelopes encrypted at rest in SQLite? What is the relay's threat model?

**G4: No requirements for Gas Town rig unavailability**
- The PRD specifies the happy path (error threshold met -> bead filed in target rig -> witness dispatches polecat) but nothing about what happens when the target rig is unreachable, the beads database is down, or bead creation fails. The bridge code (`internal/gastown/bridge.go`) exists but the PRD has no error budget or retry policy for this critical integration point.
- Why this matters: Dolt connectivity issues are explicitly called out as a known fragility in the CLAUDE.md operational guide. If faultline can't file a bead, the entire agentic loop stalls. Without a specified failure mode, the system silently drops the intent to create a bead and the error goes unaddressed.
- Suggested clarifying question: What happens when bead creation fails? Retry with backoff? Queue locally? Alert the operator? What is the acceptable failure rate for the error-to-bead bridge?

### Important Considerations

**G5: Shadow scope from DESIGN-BRIEF.md**
- DESIGN-BRIEF.md specifies organizations, teams, 5-tier RBAC (Owner/Admin/Member/Viewer/Agent), OAuth/OIDC, SSO/SAML, and a Slack integration with slash commands. None of this appears in README.md's phase definitions or architecture decisions. The main PRD says "Out of scope: SCIM / SSO / SAML authentication" (SDK-COMPATIBILITY.md line 114) while DESIGN-BRIEF.md says "P6: SSO/SAML (enterprise)."
- Why this matters: Two documents disagree on scope. Engineers reading README.md will build one system; engineers reading DESIGN-BRIEF.md will build another. The RBAC model in DESIGN-BRIEF.md fundamentally changes the data model (org table, team table, role assignments) but README.md's schema has no org concept.
- Suggested action: Reconcile DESIGN-BRIEF.md with README.md. Either promote the org/team/RBAC model into the main PRD with phase assignments, or mark it as aspirational/deferred.

**G6: No specification for Dolt commit history growth management**
- The PRD specifies 90-day event retention (`FAULTLINE_RETENTION_DAYS`), and the implementation deletes old rows. But Dolt's commit history is append-only -- deleted rows still exist in prior commits. Over time, the Dolt repository grows unboundedly even as retention purges run. Dolt has `dolt gc` (garbage collection) but it's not mentioned anywhere.
- Why this matters: A faultline instance running at 100 ev/s for 6 months will have ~1.6 billion events in its commit history even though only 90 days are in the working set. Dolt GC is the only way to reclaim space, and it requires exclusive access (no concurrent readers).
- Suggested clarifying question: Will faultline run `dolt gc` periodically? During a maintenance window? Is there a storage budget?

**G7: No multi-instance or horizontal scaling specification**
- The PRD describes "single Go binary + Dolt server" as the deployment model. But what if two faultline instances point at the same Dolt server? The sliding window tracker (in-memory, `internal/gastown/tracker.go`) and the committer (single writer, `internal/db/committer.go`) assume a single-process model. The PRD doesn't explicitly state this constraint.
- Why this matters: Users expecting to run faultline behind a load balancer (standard production pattern) will get duplicate bead creation, split-brain event counts, and conflicting Dolt commits. An explicit "single instance only" statement prevents confusion.
- Suggested action: Add a deployment constraint: "Faultline is single-instance by design. Running multiple instances against the same Dolt database is not supported."

**G8: No API versioning or deprecation policy**
- The API has two patterns: `/api/v1/projects/` (versioned) and `/api/{project_id}/issues/` (unversioned). There's no stated policy for when endpoints change, how clients discover breaking changes, or what the deprecation timeline is.
- Why this matters: Polecats and CI integrations depend on these endpoints. A breaking change to `/api/{project_id}/issues/{issue_id}/context/` (used by polecats for diagnosis) would break the agentic loop silently.
- Suggested action: Pick a versioning strategy (all under `/api/v1/`, or document the mixed approach) and add a deprecation policy, even if it's "we'll add v2 and run both for 90 days."

### Observations

**G9: No monitoring of the faultline process itself**
- README.md mentions `FAULTLINE_SELFMON_KEY` (self-monitoring) and the codebase has `internal/healthmon/`, but the PRD doesn't specify what self-monitoring means: which metrics are tracked, what alerts fire, what the health endpoint actually checks beyond "process is running." The `/health` endpoint is mentioned but its contract (what it checks, what constitutes unhealthy) is unspecified.
- Non-blocking but worth defining before relying on health checks for deployment orchestration.

**G10: No specification for concurrent issue resolution conflicts**
- Two polecats could be dispatched for related (but differently-fingerprinted) issues that share a root cause. One polecat's fix could resolve both issues, but the second polecat would still be working. The PRD's "one bead per issue group" deduplication doesn't cover cross-group correlation.
- The bead storm prevention mentioned in ADR-3 (3+ events/5min, one bead per group) handles volume but not semantic overlap. Worth noting for the Gas Town integration spec.

**G11: No error format/contract for the API**
- The API reference documents success responses but not error responses. What does a 400 look like? A 404? A 500? Are errors JSON? Is there an error code system? Polecats parsing API responses need a predictable error contract.

**G12: Retention purges Dolt working set but not lifecycle/audit data**
- The retention worker purges events, sessions, health checks, CI runs, and auth sessions. But `ft_error_lifecycle` (the audit/timeline table) has no retention policy. Over months, this table grows unboundedly. If lifecycle records are the compliance trail, they may need different retention than events.

## Confidence Assessment

**How complete does this dimension look?** Medium-High

The PRD has addressed most of the critical gaps from the dewfa round (SDK scope, commit strategy, Gas Town contracts, regression, volume, rate limiting, PII scrubbing, connection pooling, retention). The remaining gaps are in operational areas (upgrades, backup, relay security, failure modes) rather than core feature definition. These are the kinds of requirements that surface during the first real deployment rather than during design review -- which is exactly when you want them documented.

The highest-risk gap is **G3 (relay security)** because it has immediate security implications for any deployment using mobile SDKs. The highest-impact gap is **G1 (schema migration)** because it blocks every future upgrade of a running instance.
