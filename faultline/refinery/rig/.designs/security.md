# Security Analysis

## Summary

Faultline is a self-hosted Sentry-compatible error tracking system with a Go HTTP server, Dolt-backed storage, a dashboard with authentication, API token management, webhook integrations, database monitoring, and PII scrubbing. The system handles sensitive data at multiple layers: user credentials, API tokens, error payloads (which may contain PII), database connection strings for monitoring, and webhook secrets.

The overall security posture is **strong**. The codebase consistently uses secure patterns — bcrypt for passwords, AES-256-GCM for encryption at rest, parameterized SQL queries, secure cookie attributes, CSRF protection, and HMAC-SHA256 for webhook verification. The primary risks are architectural: webhook signature verification is optional (can be silently bypassed if secrets aren't configured), login rate limiting is in-memory only, and the CSP includes `unsafe-inline`.

## Analysis

### Key Considerations

- **Trust boundaries**: SDK clients (untrusted) -> ingestion endpoint (public, key-authenticated) -> storage (trusted). Dashboard users (semi-trusted, role-gated) -> API (authenticated) -> storage. Webhook senders (external) -> webhook endpoints (signature-verified when configured).
- **Attack surface**: 4 major entry points — Sentry envelope ingestion (public), REST API (authenticated), dashboard (session-authenticated), webhook endpoints (signature-verified).
- **Sensitive data at rest**: User passwords (bcrypt hashed), API tokens (SHA-256 hashed), session tokens (plaintext in DB), database monitoring connection strings (AES-256-GCM encrypted), error payloads (PII-scrubbed).
- **Multi-tenant isolation**: Project-scoped access enforced via team membership checks and token scoping. Owners/admins have cross-project visibility.

### Options Explored

#### Option 1: Accept Current Architecture (Recommended for P4)

- **Description**: The current security architecture is solid for a self-hosted, single-tenant deployment. Address the identified vulnerabilities incrementally.
- **Pros**: Already implements defense-in-depth at most layers. No major architectural rework needed.
- **Cons**: Some optional security features (webhook signatures) should be mandatory. In-memory rate limiting won't scale.
- **Effort**: Low (targeted fixes)

#### Option 2: Add Security Hardening Layer (Recommended for P5)

- **Description**: Add HSTS headers, make webhook signatures mandatory, move rate limiting to database/Redis, add audit logging, remove `unsafe-inline` from CSP.
- **Pros**: Closes all identified gaps. Production-ready security posture.
- **Cons**: Requires Redis or equivalent for distributed rate limiting. Audit logging adds storage overhead.
- **Effort**: Medium

#### Option 3: Full Enterprise Security (P6+)

- **Description**: SSO/SAML, fine-grained permissions, SOC2-style audit trail, encrypted sessions, key rotation, IP allowlisting.
- **Pros**: Enterprise-grade. Compliance-ready.
- **Cons**: Significant complexity. Premature for current stage.
- **Effort**: High

### Threat Model

| Threat | Vector | Current Mitigation | Risk Level |
|--------|--------|-------------------|------------|
| **Credential stuffing** | Dashboard login | Bcrypt + per-IP rate limiting (5/15min) | LOW (but rate limit is in-memory) |
| **API token theft** | Stolen Bearer token | SHA-256 hashed storage, project scoping, expiry | LOW |
| **SQL injection** | Any user input to DB | Parameterized queries throughout | VERY LOW |
| **XSS** | Dashboard rendering | CSP headers, `X-Content-Type-Options: nosniff` | LOW (but `unsafe-inline` in CSP) |
| **CSRF** | Dashboard state changes | Double-submit cookie pattern | VERY LOW |
| **Webhook forgery** | Unsigned webhook calls | HMAC-SHA256 verification | **MEDIUM** (optional when secret not set) |
| **PII leakage via error payloads** | Ingested events contain passwords, tokens, emails | Comprehensive regex-based scrubbing (enabled by default) | LOW |
| **Connection string exposure** | Database monitoring credentials | AES-256-GCM encryption at rest | LOW |
| **Event flooding** | Malicious SDK client | Per-project token bucket rate limiting (100/sec default) | LOW |
| **Clickjacking** | Dashboard iframe embedding | `X-Frame-Options: DENY` | VERY LOW |
| **Session hijacking** | Stolen session cookie | HttpOnly, Secure, SameSite=Strict, 30-day expiry | LOW |
| **Replay attack on webhooks** | Replayed signed webhook | Slack: 5-min timestamp window. GitHub/GasTown: no replay protection | **MEDIUM** (for non-Slack hooks) |

### Worst-Case Exploitation Scenarios

1. **Webhook forgery (if secrets unconfigured)**: Attacker sends forged resolve-hook requests to mark issues as resolved, disrupting error tracking. Forged GitHub CI webhooks could trigger false deployment signals.
2. **API token leak**: If a project-scoped token leaks, attacker can read error data for that project (potentially containing stack traces with sensitive context). Mitigated by project scoping — blast radius is one project.
3. **PII in error payloads**: If scrubbing misses a pattern, user PII could persist in Dolt storage. The scrubber covers common patterns (emails, credit cards, SSNs, bearer tokens) but custom sensitive fields may slip through.
4. **Database monitoring credential theft**: Requires API authentication + project access + compromising the encryption key (from env var or `~/.faultline/encryption.key`). Multi-layer defense makes this very difficult.

### Recommendation

**For P4 (current phase), address these critical items:**

1. **Make webhook signature verification mandatory** — Reject requests without valid signatures even when the secret is not configured. Currently, all three webhook handlers (Gas Town resolve hook, GitHub CI, Slack) silently skip verification when the secret is empty. This should fail-closed: if no secret is configured, reject all webhook requests for that integration.

2. **Add HSTS header** — Add `Strict-Transport-Security: max-age=31536000; includeSubDomains` to the security headers middleware in `internal/server/server.go`.

3. **Remove `sentry_key` query parameter support** — The ingestion auth (`internal/ingest/auth.go`) accepts the public key via URL query parameter, which risks key exposure in proxy/CDN access logs. Enforce header-only authentication (`X-Sentry-Auth` or `Authorization`).

**For P5, add:**

4. **Persistent rate limiting** — Replace in-memory login attempt tracking with database-backed or Redis-backed counters that survive restarts and work across instances.

5. **Remove `unsafe-inline` from CSP** — Move all inline scripts to external files. Use nonce-based CSP if inline scripts are unavoidable.

6. **Add audit logging** — Log privileged operations (token creation/revocation, project creation/deletion, role changes, webhook secret rotation) to a dedicated audit table.

7. **Add replay protection to non-Slack webhooks** — Add timestamp validation to Gas Town resolve hook and GitHub CI webhook handlers, similar to the Slack webhook's 5-minute window.

## Constraints Identified

- **Single-binary deployment model** means no Redis/external cache by default — rate limiting and session storage must work with Dolt or in-memory stores
- **Sentry SDK compatibility** constrains the ingestion API surface — must accept the same auth mechanisms Sentry SDKs use (including query parameter keys)
- **Self-hosted context** means the operator controls the network boundary — some mitigations (HSTS, TLS) are deployment-time concerns, not application-level
- **Dolt as sole data store** means audit logs and rate-limit counters would add commit overhead — need to balance security with Dolt performance

## Open Questions

1. **Should webhook endpoints reject unsigned requests by default?** Current behavior silently accepts when no secret is configured. Recommendation: fail-closed (reject), but this is a breaking change if operators rely on unsigned webhooks today.
2. **What is the PII scrubbing coverage target?** The current regex-based approach covers common patterns but may miss domain-specific sensitive fields. Should projects be able to configure custom scrub patterns?
3. **Session token storage**: Session tokens are stored as plaintext in the database (unlike API tokens which are hashed). Should session tokens also be hashed? Trade-off: hashing prevents session enumeration but makes session invalidation slightly more complex.
4. **Key rotation strategy**: The AES-256-GCM encryption key is static once generated. What is the plan for key rotation? Database monitoring connection strings would need re-encryption.
5. **Multi-instance rate limiting**: If faultline runs behind a load balancer with multiple instances, the in-memory rate limiter becomes ineffective. Is multi-instance deployment a P5 target?

## Integration Points

- **Authentication/Authorization** integrates with every other dimension — API design, dashboard UX, and webhook handling all depend on the auth model being correct
- **Database monitoring** introduces a new trust boundary where faultline holds credentials to external databases — this is the highest-value target for attackers and should receive proportionally more security scrutiny
- **PII scrubbing** affects data retention and compliance dimensions — scrubbing decisions are irreversible (original data is lost), so false positives (over-scrubbing) may impact debugging utility
- **Gas Town integration** means the webhook endpoint is a control plane interface — forged webhooks could disrupt the autonomous error resolution loop, not just read data
- **Sentry SDK compatibility** constrains how aggressively we can tighten ingestion auth without breaking existing SDK clients
