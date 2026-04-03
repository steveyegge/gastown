# Stakeholder Analysis

## Summary

The faultline PRD explicitly addresses three stakeholder groups well: SDK-instrumented application developers, polecats (autonomous agents consuming the API), and PMs/humans using the dashboard. However, it underserves several critical stakeholder categories: the Gas Town ecosystem actors beyond polecats (witnesses, refinery, overseer, mayor), operations teams responsible for running faultline itself, the Dolt server as a shared resource under load, and end users of the applications being monitored (whose experience degrades while errors accumulate before a polecat fix lands). There are two genuinely conflicting stakeholder needs: (1) the agent-first API design vs. human dashboard usability, where content-negotiation attempts to serve both but neither gets a first-class experience, and (2) the tension between aggressive bead creation (good for fast automated fixes) and Dolt/Gas Town resource conservation (good for system stability).

## Findings

### Critical Gaps / Questions

- **Gas Town ecosystem actors are treated as a monolith.** The PRD says "Gas Town integration" but the actual stakeholder chain is: faultline → bead creation → witness detection → polecat dispatch → polecat diagnosis (queries `/context/` API) → polecat fix → refinery merge → resolution poller confirmation. Each actor has distinct needs. The **witness** needs reliable bead metadata to decide dispatch priority. The **refinery** needs to know which beads are faultline-originated (to fast-path or special-case merge verification). The **overseer/mayor** receives escalations on 2+ regressions but has no specification for what information they receive or what actions they can take. The PRD specifies the polecat's API contract well (ADR-3) but ignores every other Gas Town actor.
  - Why this matters: If the witness can't prioritize faultline beads vs. manually-filed beads, the agentic loop's responsiveness depends on queue position. If the refinery doesn't know a merge is a faultline fix, it can't verify the error actually stopped.
  - Suggested clarifying question: "What information does each Gas Town actor (witness, refinery, overseer) need from faultline beads to do their job? Are faultline beads treated differently from manually-filed beads?"

- **No operator/admin stakeholder for faultline itself.** The PRD describes faultline as "minimal infrastructure" but someone must run it: monitor Dolt health, manage disk from accumulating events, rotate API tokens, handle relay outages, upgrade versions. There is no runbook, no operational dashboard, no alerting contract. `FAULTLINE_SELFMON_KEY` exists as a config var but its behavior is undefined. The `/health` endpoint's actual check semantics are unspecified.
  - Why this matters: Faultline is an error tracker with no specified error tracking of itself. The first operator who deploys this will immediately ask: "How do I know if faultline is broken?" and "What do I do when Dolt fills up?"
  - Suggested clarifying question: "Who operates faultline? What is their monitoring dashboard? What alerts fire when faultline is degraded?"

- **End users of monitored applications are invisible.** The entire loop — error occurs → bead filed → polecat fixes → merge — takes time. During that time, real users are hitting the error. The PRD has no concept of: user impact estimation (how many users affected?), user notification (should users know a fix is in progress?), or mitigation during the fix window (feature flags, rollback triggers, circuit breakers). The "detect → dispatch → verify → resolve" cycle assumes errors are tolerable while the fix is in flight.
  - Why this matters: For fatal errors (ruptures), the automated fix cycle could take 30+ minutes. If the monitored service is user-facing, that's 30 minutes of downtime with no mitigation path specified.
  - Suggested clarifying question: "What happens to end users while a polecat is fixing their error? Is there a mitigation strategy (rollback, feature flag) or is the assumption that errors are tolerable until the fix merges?"

### Important Considerations

- **Dolt server as a constrained shared resource.** In Gas Town, Dolt serves all beads, mail, and identity data. Faultline adds a new high-write workload (up to 100 ev/s sustained, batch-committed every 60s). The PRD treats Dolt as "MySQL on port 3307" but the Gas Town CLAUDE.md warns it is "fragile." Faultline's connection pool (25 max open, 10 idle) competes with every other Dolt consumer. There is no capacity planning or isolation strategy.
  - Why this matters: A burst of errors in a monitored application could degrade Gas Town's entire data plane — not just faultline's storage, but bead tracking, mail delivery, and agent coordination for every rig.
  - Suggested addition: Specify whether faultline uses a dedicated Dolt database (isolated) or shares the server with Gas Town system databases. Define backpressure behavior when Dolt is slow.

- **Security team requirements are partially addressed but incomplete.** PII scrubbing exists (`FAULTLINE_SCRUB_PII=true`), which is good. But: (a) the scrubbing rules aren't specified — what counts as PII? Email addresses? IP addresses? Stack trace local variables? (b) API tokens use a simple `fl_` prefix with no expiration, rotation, or scope limitation beyond project-level. (c) The relay accepts envelopes from the public internet with only DSN key authentication — there's no rate limiting at the relay level, no IP allowlisting, no abuse prevention. (d) Bearer tokens for the management API have no specified creation/rotation/revocation lifecycle beyond the endpoints existing.
  - Why this matters: Any team subject to SOC 2, HIPAA, or GDPR will need answers to these questions before deploying faultline on production services.

- **Third-party integrators / SDK maintainers.** The PRD specifies 5 target SDKs, all maintained by Sentry. Faultline's compatibility claim means tracking Sentry SDK releases — when Sentry ships a new envelope item type or changes the auth header format, faultline must respond. There's no specification for how faultline tracks SDK version compatibility or how breaking changes are communicated to users.
  - Why this matters: Sentry ships SDK updates frequently. A Sentry SDK update that changes envelope format could silently break faultline ingestion. Users would see events stop arriving with no error.
  - Suggested addition: Define which Sentry SDK versions are tested against. Add a CI matrix or compatibility test suite.

- **Developer experience for faultline contributors.** The codebase is Go with templ + HTMX. Contributing requires understanding: Go stdlib HTTP patterns, Dolt-specific SQL (dolt_add, dolt_commit), templ templating, HTMX conventions, the Gas Town bead protocol, and the Sentry envelope wire format. There's no contributor guide, no architecture walkthrough for new developers, no test harness documentation. The AGENTS.md and CLAUDE.md define CI gates but not how a human contributor would onboard.
  - Why this matters: If faultline succeeds, it needs contributors beyond the initial team. The current documentation is entirely user-facing (how to send errors) with nothing for developers (how to extend faultline).

- **Conflicting needs: aggressive detection vs. system stability.** The bead creation triggers (3 events in 5 min, or 1 fatal immediately) optimize for fast automated response. But each bead creation generates Dolt writes, triggers witness evaluation, and potentially spawns a polecat — all consuming shared Gas Town resources. During a bad deploy, one application could flood the system: 20 distinct fingerprints × 3 events each = 20 beads = 20 polecats competing for the same codebase. ADR-3 mentions "one bead per issue group" but distinct fingerprints from the same root cause create distinct beads.
  - The PRD acknowledges this (the previous review raised bead storms in Q6) but the answer — one bead per issue group — only deduplicates identical fingerprints, not correlated ones. A global rate limit on bead creation per project per time window is suggested but not specified.

- **Support team needs post-launch.** If faultline is deployed for a team's production services, someone will file support requests: "Why didn't faultline catch this error?" "Why was my error marked resolved when it's still happening?" "The polecat's fix made things worse." The PRD has no troubleshooting guide for these operational scenarios — only SDK integration troubleshooting (events not appearing, rate limits). There's no audit trail for why a bead was or wasn't created, no explanation of why an issue was marked resolved, and no rollback mechanism if a polecat fix introduces a regression.

### Observations

- **The "agent-first, human-approachable" design principle creates a real tension.** The API serves JSON to agents and HTML to humans via content negotiation. This is elegant but means neither audience gets a purpose-built experience. Agents want structured, stable, versioned JSON contracts. Humans want progressive disclosure and visual hierarchy. Content negotiation serves both adequately but neither excellently. As the product matures, these audiences will pull the API in different directions.

- **The Slack integration (planned P5/P6) is the only specified notification channel.** Teams that don't use Slack have no notification path. PagerDuty, email, Microsoft Teams, and generic webhook integrations are unmentioned. The out-of-scope list includes "Alert rules / notification routing" which means teams can't customize when they get notified or through which channel.

- **Launch coordination is implicitly handled by Gas Town itself.** Since faultline's primary consumers are Gas Town rigs, "launch" means configuring `FAULTLINE_PROJECTS` with the right rig mappings. There's no external launch coordination needed for the initial dogfood targets (gastown, myndy_ios). However, if faultline is offered to teams outside Gas Town, onboarding coordination is unspecified.

- **The MPL-2.0 license choice (ADR-6) has stakeholder implications.** Enterprise teams evaluating faultline need legal review of MPL-2.0's file-level copyleft. While less aggressive than AGPL, some enterprise legal departments flag any copyleft license. The PRD doesn't address enterprise adoption as a stakeholder concern.

- **The relay (fly.dev) is a single point of failure for all mobile/remote clients.** The relay has no specified SLA, no redundancy, no failover. If the relay goes down, all iOS/Android/remote error reporting stops silently (SDKs fire-and-forget). There's no stakeholder identified as responsible for relay uptime.

## Confidence Assessment

**Medium.** The PRD clearly identifies its primary stakeholders (SDK users, polecats, dashboard users) and serves them well. The gaps are in secondary stakeholders who become critical at production scale: operators, the broader Gas Town ecosystem, security/compliance reviewers, end users of monitored apps, and contributors. The conflicting needs between aggressive detection and system stability are acknowledged but incompletely resolved. A team could build and dogfood faultline from this PRD, but production deployment for external teams would surface most of the gaps identified above.
