# faultline

Self-hosted Sentry-compatible error tracking with [Dolt](https://github.com/dolthub/dolt) storage and [Gas Town](https://github.com/steveyegge/gastown) agentic loop integration.

[![CI](https://github.com/outdoorsea/faultline/actions/workflows/ci.yml/badge.svg)](https://github.com/outdoorsea/faultline/actions/workflows/ci.yml)
[![License: MPL-2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)

## What makes this different

- **Sentry SDK compatible** — supports browser, Node, iOS, Android, and React Native SDKs ([compatibility matrix](docs/SDK-COMPATIBILITY.md))
- **Dolt storage** — error history is version-controlled; batch commits every 60s enable time-travel queries
- **Gas Town integration** — errors automatically become beads; polecats diagnose and fix them; resolved when PR merges
- **Minimal infrastructure** — Go binary + Dolt server. No Redis, Kafka, or ClickHouse

## The loop

```
Error occurs → SDK sends envelope → faultline ingests
  → fingerprints → groups by exception type + stack frames
  → 3+ events in 5 min? → creates bead in target rig
  → witness detects → slings polecat with stack trace + context
  → polecat fixes → gt done → refinery merges
  → faultline polls bead status → marks resolved after quiet period
  → same fingerprint within 24h? → regression → reopens bead
```

## Quick start

```bash
# Requires a running Dolt server on port 3307
export FAULTLINE_DSN="root@tcp(127.0.0.1:3307)/faultline"
export FAULTLINE_PROJECTS="1:your_public_key"
export FAULTLINE_ADDR=":8080"

go run ./cmd/faultline
```

Point any Sentry SDK at `http://your_public_key@localhost:8080/1`.

## Architecture decisions

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed rationale.

| Decision | Choice | Rationale |
|----------|--------|-----------|
| SDK scope | 5 SDKs, events + sessions | [Compatibility matrix](docs/SDK-COMPATIBILITY.md) |
| Dolt commits | Batch every 60s | Balances time-travel granularity with write performance |
| Bead trigger | 3+ events/5min, error/fatal only | Prevents bead storms during bad deploys |
| Regression | 24h reopen window | Same fingerprint after resolution = regression |
| Target volume | 100 ev/s sustained | Sweet spot for teams that benefit from agentic loop |
| License | MPL-2.0 | Use freely, contribute back modifications to faultline files |

## Phases

| Phase | Name | Description | Status |
|-------|------|-------------|--------|
| P1 | Ingest | Envelope + store endpoints, DSN auth, fingerprinting, issue groups, session tracking | **Done** |
| P2 | Read API + Dolt commits | REST query endpoints, 60s batch Dolt commit loop, rate limiting (429 + X-Sentry-Rate-Limits) | **Done** |
| P3 | Gas Town integration | Bead creation (3+ events/5min), project→rig mapping, resolution polling, 24h regression, escalation | **Done** |
| P4 | Dashboard + Auth | Web UI (templ + htmx), account login, project CRUD, role-based access, issue list, event detail, stack traces | **Done** |
| P5 | Hardening | Data retention/TTL, PII scrubbing, OAuth/OIDC, API tokens, self-monitoring, Docker packaging, Slack plugin | |
| P6 | Advanced | Source maps/symbolication, Dolt time-travel queries, fingerprint tuning, full regression state machine, SSO/SAML | |

## Out of scope

These Sentry features are **not planned**:

- Session replay (recording, playback, video)
- Performance monitoring / distributed tracing
- Profiling (continuous or transaction-scoped)
- Cron monitoring / check-ins
- User feedback widgets
- Custom metrics / StatsD
- Alert rules / notification routing (Slack, PagerDuty, email)
- SCIM / SSO / SAML authentication
- Multi-region / data residency

## Stack

- **Go** — ingest API + REST API (stdlib net/http, slog)
- **Dolt** — storage (MySQL wire protocol, version-controlled)
- **templ + htmx** — dashboard (geological theme, embedded assets, zero JS build step)

## License

[Mozilla Public License 2.0](LICENSE) — use freely, modifications to faultline source files must be shared.
