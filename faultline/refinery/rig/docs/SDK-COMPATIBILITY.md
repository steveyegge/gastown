# SDK Compatibility

> This document defines faultline's Sentry SDK compatibility surface.
> It is the authoritative reference for what is supported, what is planned,
> and what is explicitly out of scope.

## License

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.

## Target SDKs

| SDK | Platform | Status |
|-----|----------|--------|
| @sentry/browser | Web (browser) | MVP |
| @sentry/node | Web (server) | MVP |
| sentry-cocoa | iOS / macOS | MVP |
| sentry-android | Android | MVP |
| sentry-react-native | React Native | MVP |

## Endpoints

| Endpoint | Status | Notes |
|----------|--------|-------|
| `POST /api/{project_id}/envelope/` | **Supported** | Primary ingest endpoint |
| `POST /api/{project_id}/store/` | **Supported** | Legacy single-event JSON |
| `GET /health` | **Supported** | Health check |
| `POST /api/{project_id}/minidump/` | Not planned | Used by sentry-native (C/C++) only |
| `POST /api/{project_id}/unreal/` | Not planned | Unreal Engine only |
| `POST /api/{project_id}/security/` | Not planned | Browser CSP report-uri |

## Authentication

SDKs authenticate via DSN public key. Faultline checks these sources in order:

1. `X-Sentry-Auth` header: `Sentry sentry_key={KEY}, sentry_version=7`
2. `Authorization` header (same format)
3. `sentry_key` query parameter

DSN format: `https://{PUBLIC_KEY}@{HOST}/{PROJECT_ID}`

## Envelope Item Types

### Processed (stored in Dolt)

| Item Type | Description | Default in SDKs | Since |
|-----------|-------------|-----------------|-------|
| `event` | Error events, crashes | All 5 SDKs | P1 |
| `session` | Individual session updates | All 5 SDKs | P1.1 |
| `sessions` | Aggregated session batches (Node server-mode) | @sentry/node | P1.1 |

### Accepted and Silently Dropped

These item types are accepted (200 OK returned) but not stored.
SDKs fire-and-forget all items — silent drop causes no client-side errors.

| Item Type | Description | Why dropped |
|-----------|-------------|-------------|
| `client_report` | SDK discarded-event statistics | Internal SDK metrics, no user value |
| `transaction` | Performance monitoring spans | Out of scope for MVP |
| `span` | Standalone performance spans | Out of scope for MVP |
| `attachment` | File attachments (screenshots, view hierarchy) | Out of scope for MVP |
| `profile` | Profiling data | Out of scope for MVP |
| `profile_chunk` | Continuous profiling chunks | Out of scope for MVP |
| `replay_event` | Session replay metadata | Out of scope for MVP |
| `replay_recording` | Session replay DOM recording | Out of scope for MVP |
| `replay_video` | Mobile session replay video | Out of scope for MVP |
| `user_report` | User feedback (legacy) | Out of scope for MVP |
| `feedback` | User feedback (new) | Out of scope for MVP |
| `check_in` | Cron monitor check-ins | Out of scope for MVP |
| `raw_security` | CSP/security policy reports | Out of scope for MVP |
| `log` | Structured log entries | Experimental |
| `metric` / `statsd` | Custom metrics | Experimental |
| `trace_metric` | Metric extraction from traces | Experimental |

### Response Format

**Success (200):**
```json
{"id": "<event_id>"}
```

**Rate limited (429):**
```
HTTP/1.1 429 Too Many Requests
Retry-After: 60
X-Sentry-Rate-Limits: 60::project:quota_exceeded
```

SDKs parse `X-Sentry-Rate-Limits` first, fall back to `Retry-After`, default to 60s backoff.

## Mobile-Specific Notes

- **Crash reports:** iOS and Android SDKs capture crashes in-process via signal handlers and send them as regular `event` items on next app launch. No minidump endpoint needed.
- **Symbolication:** Mobile events include `debug_meta` with debug image references. Faultline stores this in raw JSON but does not perform server-side symbolication. Stack traces appear with raw addresses — symbolication is deferred.
- **Offline caching:** Mobile SDKs cache events to disk and send on next launch. Faultline's idempotent `INSERT IGNORE` on `event_id` handles retries correctly.

## Explicit Out-of-Scope

The following Sentry features are **not planned** for any phase:

- Session replay (recording, playback, video)
- Server-side symbolication / source map processing
- Performance monitoring / distributed tracing
- Profiling (continuous or transaction-scoped)
- Cron monitoring / check-ins
- User feedback widgets
- Release health tracking
- Custom metrics / StatsD ingestion
- Alert rules / notification routing (Slack, PagerDuty, email)
- SCIM / SSO / SAML authentication
- Multi-region / data residency

These may be reconsidered in future phases based on demand.

## Size Limits

| Limit | Value |
|-------|-------|
| Max envelope size | 200 MiB |
| Max item size | 1 MiB |
| Max event_id length | 36 characters (UUID) |

## Compression

Faultline supports `Content-Encoding: gzip` transparently. All SDKs compress
envelopes by default.
