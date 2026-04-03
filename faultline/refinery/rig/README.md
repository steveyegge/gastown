# faultline

Self-hosted Sentry-compatible error tracking with [Dolt](https://github.com/dolthub/dolt) storage and [Gas Town](https://github.com/steveyegge/gastown) agentic loop integration.

[![CI](https://github.com/outdoorsea/faultline/actions/workflows/ci.yml/badge.svg)](https://github.com/outdoorsea/faultline/actions/workflows/ci.yml)
[![License: MPL-2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)

## What makes this different

- **Sentry SDK compatible** -- use any of 5 supported Sentry SDKs (browser, Node, iOS, Android, React Native) with zero code changes ([compatibility matrix](docs/SDK-COMPATIBILITY.md))
- **Dolt storage** -- error history is version-controlled; batch commits every 60s enable time-travel queries
- **Gas Town integration** -- errors automatically become beads; polecats diagnose and fix them autonomously; resolution confirmed when the fix merges and the error stops recurring
- **Minimal infrastructure** -- single Go binary + Dolt server. No Redis, Kafka, or ClickHouse

## The loop

```
Error occurs --> SDK sends envelope --> faultline ingests
  --> fingerprints --> groups by exception type + stack frames
  --> 3+ events in 5 min? --> creates bead in target rig
  --> witness detects --> dispatches polecat with stack trace + context
  --> polecat fixes --> gt done --> refinery merges
  --> faultline polls bead status --> marks resolved after quiet period
  --> same fingerprint within 24h? --> regression --> new bead filed
```

## Quick start

### Prerequisites

- [Go 1.26+](https://golang.org/dl/)
- [Dolt](https://github.com/dolthub/dolt) server running on port 3307

### Install and run

```bash
git clone https://github.com/outdoorsea/faultline.git
cd faultline

# Set required environment variables
export FAULTLINE_DSN="root@tcp(127.0.0.1:3307)/faultline"
export FAULTLINE_PROJECTS="1:your_public_key"
export FAULTLINE_ADDR=":8080"

# Run the server
go run ./cmd/faultline
```

Open the dashboard at `http://localhost:8080/dashboard/` and follow the first-run setup to create your account.

### Register a project

Projects listed in `FAULTLINE_PROJECTS` are seeded automatically. To register additional projects at runtime (no restart required):

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Authorization: Bearer <api-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "my_project", "rig": "my_rig"}'
```

Response:

```json
{
  "project_id": 123,
  "dsn": "http://abc123@localhost:8080/123",
  "public_key": "abc123"
}
```

### Send your first error

Point any Sentry SDK at your faultline instance using the DSN:

```
http://your_public_key@localhost:8080/1
```

```python
import sentry_sdk
sentry_sdk.init(dsn="http://your_public_key@localhost:8080/1", traces_sample_rate=0)
raise ValueError("test error")
```

The error appears on the dashboard within seconds.

## SDK integration

Faultline is fully Sentry-compatible. Use the standard Sentry SDK for your language -- just point the DSN at your faultline instance.

**Important:** Always set `traces_sample_rate=0` and disable tracing. Faultline processes error events only; performance traces are silently dropped.

### Go

```bash
go get github.com/outdoorsea/faultline/pkg/gtfaultline
```

```go
import "github.com/outdoorsea/faultline/pkg/gtfaultline"

gtfaultline.Init(gtfaultline.Config{
    DSN:         os.Getenv("FAULTLINE_DSN"),
    Release:     version,
    Environment: os.Getenv("GT_ENV"),
    URL:         "http://localhost:3000", // auto-registers in dashboard
})
defer gtfaultline.Flush(2 * time.Second)
defer gtfaultline.RecoverAndReport()
```

The Go SDK automatically sends a heartbeat on startup and supports panic recovery via `RecoverAndReport()`.

### Python

```bash
pip install sentry-sdk
```

```python
import sentry_sdk
import os

sentry_sdk.init(
    dsn=os.environ.get("FAULTLINE_DSN"),
    environment=os.environ.get("ENV", "development"),
    traces_sample_rate=0,
    enable_tracing=False,
)
```

Framework integrations work out of the box: `sentry-sdk[fastapi]`, `sentry-sdk[flask]`, `sentry-sdk[django]`, `sentry-sdk[celery]`.

### Node.js / TypeScript

```bash
npm install @sentry/node
```

```typescript
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: process.env.FAULTLINE_DSN,
  environment: process.env.NODE_ENV,
  tracesSampleRate: 0,
});
```

For Next.js, use `@sentry/nextjs` and set the client-side DSN via `NEXT_PUBLIC_FAULTLINE_DSN`.

### Swift (iOS / macOS)

```swift
import Sentry

SentrySDK.start { options in
    options.dsn = "https://KEY@faultline.live/PROJECT_ID"
    options.environment = "production"
    options.enableAutoSessionTracking = true
    options.attachStacktrace = true
}
```

Mobile apps must use the relay DSN since they cannot reach localhost. See [Relay](#relay) below.

### Heartbeat

Sentry SDKs are passive -- they only fire on errors. A healthy service sends nothing, so faultline can't tell if it's running. Add a heartbeat to register your service as active.

**Endpoint:** `POST /api/{project_id}/heartbeat`

The Go SDK sends a heartbeat automatically on `Init()`. For other languages, see [docs/HEARTBEAT.md](docs/HEARTBEAT.md) for per-language examples.

If the heartbeat body includes a `"url"` field, faultline saves it to the project config so the dashboard can link directly to your service.

## Dashboard

The web UI is built with templ + HTMX (no JavaScript build step) and uses a geological theme.

### Projects page (`/dashboard/`)

Lists all registered projects with at-a-glance status:

- **Status indicators** -- green (healthy), yellow (errors detected), red (critical)
- **Error rate sparkline** -- 24-hour event distribution
- **Uptime percentage** -- 24-hour availability from health checks
- **Event and issue counts** -- total events, unresolved issues
- **Platform icons** -- Go, JavaScript, iOS, Android, etc.
- Auto-refreshes every 60 seconds

### Issue list (seismograph)

- Issues grouped by fingerprint and sorted by severity or recency
- Geological severity scale: tremor, quake, rupture, aftershock, dormant
- Status filters: Active, Fixing, Stabilized, All
- Environment filter for multi-environment projects
- Event count, first/last seen, culprit (top stack frame)

### Issue detail

- Full exception type and message
- Stack trace display
- Environment and release tags
- Raw event JSON
- Lifecycle timeline (detection, bead filed, dispatched, resolved, regressed)
- Linked bead ID for Gas Town tracking
- Manual resolve and dispatch buttons

### Project settings (`/dashboard/projects/{id}/settings`)

| Field | Description | Example |
|-------|-------------|---------|
| **Description** | Short project label | "Gas Town coordinator" |
| **URL** | Service URL (auto-set by heartbeat) | `http://localhost:8000` |
| **Deployment type** | `local`, `remote`, or `hosted` | `local` |
| **Components** | Comma-separated service parts | `web, api, database` |
| **Environments** | Comma-separated env names | `staging, production` |

## Agent-first workflow

Faultline's distinguishing feature is the autonomous error resolution lifecycle powered by Gas Town.

### Detect

- SDK sends error events to faultline via the Sentry envelope protocol
- Events are fingerprinted (exception type + top 3 stack frames) and grouped into issue groups
- A sliding-window tracker monitors event counts per fingerprint over a 5-minute window

### Dispatch

When a threshold is met, faultline files a bead in the target rig:

| Trigger | Condition |
|---------|-----------|
| **Burst** | 3+ error/fatal events in 5 minutes |
| **Fatal** | 1 fatal event (immediate) |
| **Slow burn** | Any error unbeaded for >1 hour |

The bead includes the exception type, stack trace culprit, event count, sample raw event, and a link to the faultline API for full context.

### Verify

- The rig's witness detects the new bead and dispatches a polecat
- The polecat queries `GET /api/{project_id}/issues/{issue_id}/context/` for stack traces, event history, and CI run data
- The polecat diagnoses the bug, implements a fix, and submits via `gt done`
- The refinery merges the fix to main

### Resolve

- Faultline polls bead status every 60 seconds
- When the bead is closed, a 10-minute quiet period begins
- If no new events arrive during the quiet period, the issue is marked resolved
- If events arrive during the quiet period, resolution is cancelled

### Regression (aftershock)

- Same fingerprint reappearing within 24 hours of resolution triggers a regression
- A new bead is filed (not a reopen) with the `error:regression` label
- 2+ regressions on the same issue escalate to the crew/overseer

### Lifecycle events

Every stage is recorded in the `ft_error_lifecycle` table: detection, bead_filed, dispatched, resolved, regressed, escalation. The issue detail page shows these as a timeline.

## Configuration

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FAULTLINE_ADDR` | `:8080` | HTTP listen address |
| `FAULTLINE_DSN` | `root@tcp(127.0.0.1:3307)/faultline` | Dolt connection string (MySQL format) |
| `FAULTLINE_PROJECTS` | (defaults) | Comma-separated `id:public_key[:rig]` |
| `FAULTLINE_RATE_LIMIT` | `100` | Max events per second per project |
| `FAULTLINE_RETENTION_DAYS` | `90` | Event and session TTL in days |
| `FAULTLINE_SCRUB_PII` | `true` | Server-side PII removal |
| `FAULTLINE_API_URL` | `http://localhost:8080` | Base URL for generated DSNs |
| `FAULTLINE_SLACK_WEBHOOK` | (empty) | Slack incoming webhook URL |
| `FAULTLINE_CI_WEBHOOK_SECRET` | (empty) | HMAC-SHA256 secret for GitHub/resolve webhooks |
| `FAULTLINE_RELAY_URL` | `https://faultline.live` | Public relay URL |
| `FAULTLINE_RELAY_POLL_SECS` | `30` | Relay poll interval in seconds |
| `FAULTLINE_HEALTHMON_DOCTOR` | `false` | Run Dolt health diagnostics |
| `FAULTLINE_UPTIME_INTERVAL_SECS` | `60` | Health check interval in seconds |
| `FAULTLINE_SELFMON_KEY` | (first key) | Project key for self-monitoring |

### Environments

Set the `environment` field in your SDK init to tag errors by deployment stage. Use the **same DSN** for all environments -- the environment is a tag on the event, not a separate project.

```python
sentry_sdk.init(dsn=os.environ["FAULTLINE_DSN"], environment="staging")
```

Configure known environments in project settings (comma-separated). The dashboard shows environment-specific filters. Events from unlisted environments are still accepted.

### DSN format

```
http://{PUBLIC_KEY}@{HOST}:{PORT}/{PROJECT_ID}
```

| Deployment | DSN |
|------------|-----|
| **Local** | `http://KEY@localhost:8080/PROJECT_ID` |
| **Docker** | `http://KEY@host.docker.internal:8080/PROJECT_ID` |
| **Remote/Mobile** | `https://KEY@faultline.live/PROJECT_ID` |

### Docker

```bash
docker build -t faultline .
docker run -p 8080:8080 \
  -e FAULTLINE_DSN="root@tcp(host.docker.internal:3307)/faultline" \
  -e FAULTLINE_PROJECTS="1:your_key" \
  faultline
```

### Daemon mode

```bash
faultline start    # Run as background daemon
faultline status   # Check health and PID
faultline stop     # Stop the daemon
faultline serve    # Run in foreground (default)
```

## API reference

All management API endpoints require a Bearer token (`Authorization: Bearer <token>`). Generate tokens via `/api/{project_id}/tokens/`.

### Ingest (Sentry protocol)

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/{project_id}/envelope/` | POST | DSN key | Primary ingest (Sentry v7 envelope format) |
| `/api/{project_id}/store/` | POST | DSN key | Legacy single-event JSON |
| `/api/{project_id}/heartbeat` | POST | DSN key | Liveness ping (no event created) |

Authentication: `X-Sentry-Auth: Sentry sentry_key={KEY}, sentry_version=7` header, `Authorization` header, or `sentry_key` query parameter.

### Projects

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/projects/` | GET | List all projects |
| `/api/v1/register` | POST | Create project (returns DSN) |
| `/api/{project_id}/dsn/` | GET | Get project DSN |

### Issues

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/{project_id}/issues/` | GET | List issues for project |
| `/api/{project_id}/issues/{issue_id}/` | GET/PUT/PATCH | Get or update issue |
| `/api/{project_id}/issues/{issue_id}/events/` | GET | List events for issue |
| `/api/{project_id}/issues/{issue_id}/context/` | GET | Stack traces and context (used by polecats) |
| `/api/{project_id}/issues/{issue_id}/lifecycle/` | GET | Lifecycle event timeline |
| `/api/{project_id}/issues/{issue_id}/dolt-log/` | GET | Dolt commit history |
| `/api/{project_id}/issues/{issue_id}/history/` | GET | Historical snapshots |
| `/api/{project_id}/issues/{issue_id}/as-of/` | GET | Time-travel query (Dolt) |
| `/api/{project_id}/issues/{issue_id}/assign-bead/` | POST | Link bead to issue |
| `/api/{project_id}/events/{event_id}/` | GET | Fetch single event |

### Fingerprint rules

Custom rules for error grouping, applied per-project by priority.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/{project_id}/fingerprint-rules/` | GET/POST | List or create rules |
| `/api/{project_id}/fingerprint-rules/{rule_id}/` | GET/PUT/DELETE | Manage rule |

Rules can match on `exception_type`, `message`, `module`, or `tag`.

### API tokens

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/{project_id}/tokens/` | GET/POST | List or create tokens (`fl_` prefix) |
| `/api/{project_id}/tokens/{token_id}/` | DELETE | Revoke token |

### Source maps

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/{project_id}/sourcemaps/` | GET/POST | List or upload source maps |
| `/api/{project_id}/sourcemaps/{id}/` | DELETE | Delete source map |

### Webhooks

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/hooks/ci/github` | POST | HMAC-SHA256 | GitHub Actions workflow events |
| `/api/hooks/resolve/` | POST | HMAC-SHA256 | Bead resolution notification |

**GitHub Actions webhook:** Converts CI failures into faultline events and stores successes for fix verification. Set `FAULTLINE_CI_WEBHOOK_SECRET` and configure a repository webhook pointing at this endpoint.

**Slack notifications:** Set `FAULTLINE_SLACK_WEBHOOK` to receive Block Kit notifications for new issues, resolutions, regressions, and escalations.

## Architecture

### Stack

| Layer | Technology |
|-------|------------|
| **Backend** | Go (stdlib `net/http`, `slog`) |
| **Storage** | Dolt (MySQL wire protocol, version-controlled) |
| **Frontend** | templ + HTMX (embedded assets, zero JS build step) |
| **Relay** | Separate Go binary, SQLite storage, deployed on Fly.dev |

### Data flow

```
SDK (error) --> /api/{id}/envelope/ --> Parse envelope --> Authenticate (DSN key)
  --> PII scrubbing --> Fingerprinting --> Issue grouping --> Insert into Dolt
  --> Committer batches writes every 60s --> Dolt commit

  --> Gas Town bridge: OnEvent callback
  --> Sliding window tracker (5 min) --> Threshold check
  --> Bead creation in target rig --> Witness dispatches polecat
  --> Polecat queries /api/.../context/ --> Fixes bug --> Refinery merges
  --> Resolution poller checks bead status --> Quiet period --> Resolved
```

### Relay

For mobile apps, desktop apps, and hosted services that can't reach localhost, faultline provides a public relay:

```
+-------------+       +---------------------+       +------------------+
|  Your App   |------>|  Relay (fly.dev)     |<------|  Faultline :8080 |
|  (SDK)      |       |  SQLite, 7-day TTL   | poll  |  (local poller)  |
+-------------+       +---------------------+       +------------------+
```

The relay stores envelopes in SQLite with a 7-day TTL. The local faultline instance polls `GET /relay/poll` every 30 seconds, processes envelopes through the normal ingest pipeline, and acknowledges with `POST /relay/ack`.

The relay starts automatically when `FAULTLINE_RELAY_URL` is set.

### Dolt storage

Faultline uses Dolt as its database, providing MySQL-compatible SQL with git-like version control:

- **Batch commits:** Writes are batched and committed every 60 seconds (`CALL dolt_add('-A')` then `CALL dolt_commit`)
- **Time-travel:** Query any issue's state at a past commit via the `/as-of/` endpoint
- **Commit log:** View the Dolt commit history for any issue via `/dolt-log/`
- **Connection pool:** 25 max open connections, 10 idle, 5 min max lifetime

### Gas Town integration

Faultline maps projects to Gas Town rigs via the `FAULTLINE_PROJECTS` env var:

```
FAULTLINE_PROJECTS="1:key1:faultline,2:key2:myapp"
```

Format: `project_id:public_key:rig_name`. The rig name tells faultline where to file beads when errors are detected in that project.

## Architecture decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| SDK scope | 5 SDKs, events + sessions | [Compatibility matrix](docs/SDK-COMPATIBILITY.md) |
| Dolt commits | Batch every 60s | Balances time-travel granularity with write performance |
| Bead trigger | 3+ events/5min, error/fatal only | Prevents bead storms during bad deploys |
| Regression | 24h reopen window | Same fingerprint after resolution = regression |
| Target volume | 100 ev/s sustained | Sweet spot for teams that benefit from agentic loop |
| License | MPL-2.0 | Use freely, contribute back modifications to faultline files |

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed ADRs.

## Out of scope

These Sentry features are **not planned**:

- Session replay (recording, playback, video)
- Performance monitoring / distributed tracing
- Profiling (continuous or transaction-scoped)
- Cron monitoring / check-ins
- User feedback widgets
- Custom metrics / StatsD
- Multi-region / data residency

## Troubleshooting

**Events not appearing:** Verify the DSN is correct and the service can reach faultline. Test with:
```bash
curl -X POST http://localhost:8080/api/1/heartbeat \
  -H "X-Sentry-Auth: Sentry sentry_key=YOUR_KEY"
```

**"Unknown event" warnings:** You have `traces_sample_rate > 0`. Set it to `0` -- faultline only processes error events.

**Rate limited (429):** Your project is exceeding the per-project event rate limit (default 100/s). Increase `FAULTLINE_RATE_LIMIT` or investigate the error volume.

**SENTRY_DSN vs FAULTLINE_DSN:** Both work. Use `FAULTLINE_DSN` to avoid confusion with external Sentry instances. The SDK only cares about the DSN value, not the env var name.

**Docker networking:** Use `host.docker.internal` to reach faultline from inside a container. On Linux, add `extra_hosts: ["host.docker.internal:host-gateway"]` to your docker-compose.yml.

## License

[Mozilla Public License 2.0](LICENSE) -- use freely, modifications to faultline source files must be shared.
