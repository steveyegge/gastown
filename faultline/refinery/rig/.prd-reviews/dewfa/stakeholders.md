# Stakeholder Analysis

## Summary

Faultline positions itself as a self-hosted, Sentry-compatible error tracker with Dolt storage and Gas Town agentic loop integration. The PRD (README) focuses almost entirely on the developer-as-user and the agentic repair loop, but leaves several critical stakeholder groups unaddressed. Most notably, the operations/SRE team who will run faultline in production, the security team who must evaluate a new ingest surface, and the existing Sentry SDK ecosystem maintainers whose compatibility contract faultline implicitly adopts are all absent from the document.

The stakeholder gaps are significant because faultline sits at a critical intersection: it receives error telemetry (potentially containing PII, secrets, and stack traces), it creates work items that trigger autonomous code changes (polecats), and it stores everything in a version-controlled database where "every error event is a git commit." Each of these properties attracts a different stakeholder with different — sometimes conflicting — requirements.

## Findings

### Critical Gaps / Questions

- **Operations / SRE team is entirely absent.** Faultline will need to be deployed, monitored, scaled, and maintained. The PRD mentions "zero extra infrastructure" but doesn't address: Who monitors faultline itself? What happens when Dolt storage fills up? What are the resource requirements (CPU, memory, disk) per event volume? How is faultline restarted or upgraded without losing in-flight events?
  - Why this matters: An error tracking system that goes down during an incident is the worst possible failure mode. Ops needs runbooks, alerting, capacity planning.
  - Suggested question: What is the expected event volume, and what are the operational requirements for running faultline reliably?

- **Security team has no visibility into the threat surface.** Faultline accepts arbitrary JSON payloads from any Sentry SDK client, authenticated only by a symmetric key in an environment variable. The raw JSON (which may contain PII, credentials leaked in error messages, or stack traces with sensitive paths) is stored verbatim in Dolt.
  - Why this matters: Error payloads are a known vector for credential leakage. A system that stores raw events indefinitely with no scrubbing, no access controls, and no retention policy is a compliance liability.
  - Suggested question: What is the data classification of stored error events, and what scrubbing/retention/access controls are required before production use?

- **The Sentry SDK ecosystem is an implicit dependency with no compatibility contract.** The PRD says "drop-in replacement for any Sentry SDK" but the implementation only handles `event`, `transaction`, and `error` envelope item types. Sessions, attachments, profiles, replays, check-ins, and other item types are silently dropped.
  - Why this matters: SDK users will configure their Sentry DSN to point at faultline and expect full functionality. Silent data loss (sessions, attachments) will cause confusion and erode trust.
  - Suggested question: Which Sentry SDK versions and features are explicitly supported? Should unsupported item types return errors or be silently dropped?

### Important Considerations

- **Gas Town polecats (autonomous agents) are a downstream stakeholder with unique needs.** The PRD describes a loop where errors become beads and polecats fix them. But the current implementation has no query API — polecats can't retrieve issue details, stack traces, or event history. The polecat-as-consumer is mentioned but not designed for.
  - The bead creation mechanism (P3) and the query API that polecats would use to understand errors are unspecified. What data does a polecat need to effectively diagnose and fix a bug from an error event?

- **Dashboard users (P3) are mentioned but have zero requirements.** "React/Vite dashboard" appears in the phase list with no description of who uses it, what they need to see, or how it differs from Sentry's UI. Is this for developers triaging errors? For managers tracking error rates? For ops monitoring system health?

- **Third-party integrators and notification consumers are not mentioned.** Error tracking systems typically integrate with Slack, PagerDuty, Jira, email, and webhooks. The PRD's agentic loop (Gas Town) is a novel alternative, but many teams will still need conventional alerting.
  - Who gets notified when a new error group appears? Only the Gas Town witness, or also human on-call?

- **Multi-tenant / multi-project isolation is underspecified.** The auth model maps project IDs to keys, but there's no concept of organizations, teams, or access scoping. If multiple teams use the same faultline instance, can team A see team B's errors? The current answer is "yes, if they have any valid key" — there's no project-scoped query isolation.

- **Dolt server administrators** need to understand that faultline will create a commit per event upsert (based on "every error event is a git commit"). At high event volumes, this could create enormous commit histories. Dolt's performance characteristics under this write pattern are unvalidated.

### Observations

- **The "zero extra infrastructure" claim conflicts with the Dolt dependency.** Dolt is specialized infrastructure that most teams don't already run. The claim should be qualified or the dependency should be acknowledged as a prerequisite, not dismissed.

- **Developer experience for SDK users is a strength but underdocumented.** The Sentry SDK compatibility means zero client-side changes, which is excellent. But there's no documentation on: which SDKs have been tested, what DSN format to use, or what behavioral differences to expect vs. real Sentry.

- **The support team (if one exists) has no tools.** There's no admin API, no way to replay events, no way to manually group/ungroup issues, no way to delete sensitive events. Post-launch support requests like "delete this event that contains a customer's password" have no path to resolution.

- **Compliance / legal: data residency and retention are unaddressed.** If faultline stores error events in Dolt with no TTL or deletion mechanism, GDPR right-to-deletion requests and data retention policies cannot be satisfied. This is a blocker for any team subject to data protection regulations.

- **Internal team dependencies:** The Dolt team (or whoever maintains the Dolt server) needs to be aware that faultline is a new, potentially high-volume consumer. The Gas Town team needs to design the bead creation interface (P3). Neither dependency is acknowledged.

## Confidence Assessment

**Medium-Low.** The PRD is a brief README that describes the vision and architecture but lacks the detail needed for a thorough stakeholder analysis. Key gaps: no target users or personas are defined, no deployment context is described, no scale targets are given, no compliance requirements are stated. The stakeholder analysis above is based on inferring stakeholders from the architecture — a more complete PRD would name them explicitly and describe their needs.
