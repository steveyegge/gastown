# Faultline Integration Guide

How to connect any project to faultline for error tracking. Faultline is
fully Sentry-compatible — use the standard Sentry SDK for your language.

## Quick Start (3 steps)

### 1. Register your project

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Authorization: Bearer <api-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "my_project", "rig": "my_rig"}'
```

Response includes your DSN. No server restart needed.

### 2. Install SDK + set DSN

**Python (FastAPI, Flask, Celery, Django):**
```bash
pip install sentry-sdk[fastapi]  # or [flask], [celery], [django]
```

```python
import sentry_sdk
import os

sentry_sdk.init(
    dsn=os.environ.get("FAULTLINE_DSN"),
    environment=os.environ.get("ENV", "development"),
    traces_sample_rate=0,   # disable performance tracing (not supported)
    enable_tracing=False,   # explicit: faultline only processes errors
)
```

**Node.js / TypeScript:**
```bash
npm install @sentry/node  # or @sentry/nextjs for Next.js
```

```typescript
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: process.env.FAULTLINE_DSN,
  environment: process.env.NODE_ENV,
  tracesSampleRate: 0,  // disable performance tracing
});
```

**Go:**
```bash
go get github.com/outdoorsea/faultline/pkg/gtfaultline
```

```go
import "github.com/outdoorsea/faultline/pkg/gtfaultline"

gtfaultline.Init(gtfaultline.Config{
    DSN:         os.Getenv("FAULTLINE_DSN"),
    Release:     version,
    Environment: os.Getenv("GT_ENV"),
})
defer gtfaultline.Flush(2 * time.Second)
defer gtfaultline.RecoverAndReport()
```

**Swift (iOS):**
```swift
import Sentry

SentrySDK.start { options in
    options.dsn = "https://KEY@faultline-relay.fly.dev/PROJECT_ID"
    options.environment = "production"
    options.enableAutoSessionTracking = true
    options.attachStacktrace = true
}
```

### 3. Add heartbeat

Sentry SDKs are passive — they only fire on errors. A healthy service sends
nothing, so faultline can't tell if it's running. Add a heartbeat.

The heartbeat also supports **URL auto-registration**: if the body contains a
`"url"` field, faultline saves it to the project config. This lets the dashboard
link directly to your service without manual configuration.

**Endpoint:** `POST /api/{project_id}/heartbeat`

**Go:** Automatic — `gtfaultline.Init()` sends a heartbeat on startup with
the project URL included, so your service appears on the dashboard immediately.

**Python:**
```python
import threading, time, requests
from urllib.parse import urlparse

def _faultline_heartbeat():
    dsn = os.environ.get("FAULTLINE_DSN", "")
    if not dsn:
        return
    parsed = urlparse(dsn)
    base = f"{parsed.scheme}://{parsed.hostname}" + (f":{parsed.port}" if parsed.port else "")
    url = f"{base}/api/{parsed.path.strip('/')}/heartbeat"
    headers = {"X-Sentry-Auth": f"Sentry sentry_key={parsed.username}"}
    # Optional: include service URL for auto-registration
    body = {"url": "http://localhost:8000"}  # your service URL
    while True:
        try:
            requests.post(url, headers=headers, json=body, timeout=5)
        except Exception:
            pass
        time.sleep(300)

threading.Thread(target=_faultline_heartbeat, daemon=True).start()
```

**Node.js:**
```typescript
function faultlineHeartbeat() {
  const dsn = process.env.FAULTLINE_DSN;
  if (!dsn) return;
  const url = new URL(dsn);
  const base = `${url.protocol}//${url.host}`;
  const projectId = url.pathname.replace(/\//g, "");
  fetch(`${base}/api/${projectId}/heartbeat`, {
    method: "POST",
    headers: {
      "X-Sentry-Auth": `Sentry sentry_key=${url.username}`,
      "Content-Type": "application/json",
    },
    // Optional: include service URL for auto-registration
    body: JSON.stringify({ url: "http://localhost:3000" }),
  }).catch(() => {});
}
faultlineHeartbeat();
setInterval(faultlineHeartbeat, 300_000);
```

See [docs/HEARTBEAT.md](HEARTBEAT.md) for all languages and the one-shot variant.

## Important: What faultline does NOT support

Faultline only processes **error events**. Everything else is silently accepted
(returns 200 OK) but not stored:

- Performance traces / transactions (`traces_sample_rate`)
- Session replays
- Profiling
- Metrics / custom instrumentation

**Always set `traces_sample_rate=0` and `enable_tracing=False`** to avoid
sending unnecessary data.

## DSN Configuration

### Environment variable

Use `FAULTLINE_DSN`. If your project already uses `SENTRY_DSN`, both work —
they're the same Sentry protocol. `FAULTLINE_DSN` is the canonical name to
avoid confusion with external Sentry instances.

### Local services (can reach localhost)

```
FAULTLINE_DSN=http://KEY@localhost:8080/PROJECT_ID
```

### Docker services

```
FAULTLINE_DSN=http://KEY@host.docker.internal:8080/PROJECT_ID
```

**Linux Docker note:** `host.docker.internal` may not resolve by default. Add to
docker-compose.yml:
```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

### Remote services (mobile apps, desktop apps, hosted services)

Use the public relay:
```
FAULTLINE_DSN=https://KEY@faultline-relay.fly.dev/PROJECT_ID
```

The relay stores events and the local faultline polls them every 30 seconds.

## Environments

Set the `environment` field in your SDK init to tag errors by deployment stage.
Faultline stores this per-event — you can filter the issue list by environment
on the dashboard.

**Staging vs production DSNs:** Use the **same DSN** for both. Environments are
a tag on the event, not a separate project. The Sentry SDK sends the environment
string with every error:

```python
sentry_sdk.init(
    dsn=os.environ.get("FAULTLINE_DSN"),
    environment="staging",  # or "production", "development"
)
```

```typescript
Sentry.init({
  dsn: process.env.FAULTLINE_DSN,
  environment: "production",
});
```

```go
gtfaultline.Init(gtfaultline.Config{
    DSN:         os.Getenv("FAULTLINE_DSN"),
    Environment: "production",
})
```

**Configuring known environments:** In the project settings page
(`/dashboard/projects/{id}/settings`), list your environments as
comma-separated values (e.g. `staging, production`). This helps the dashboard
show environment-specific filters. Events from unlisted environments are still
accepted — the list is advisory, not enforced.

## Architecture: Relay vs Direct

Faultline supports two ingestion paths depending on where your service runs.

### Direct (local services)

```
┌─────────────┐       ┌──────────────────┐
│  Your App   │──────▶│  Faultline :8080  │
│ (SDK + DSN) │       │  (local server)   │
└─────────────┘       └──────────────────┘
```

SDKs send errors directly to faultline on `localhost:8080`. This is the
default for services running on the same machine or in Docker (via
`host.docker.internal`).

**DSN:** `http://KEY@localhost:8080/PROJECT_ID`

### Via relay (remote / mobile / hosted services)

```
┌─────────────┐       ┌─────────────────────┐       ┌──────────────────┐
│  Your App   │──────▶│  Relay (fly.dev)     │◀──────│  Faultline :8080  │
│ (SDK + DSN) │       │  stores envelopes    │ poll  │  (local poller)   │
└─────────────┘       └─────────────────────┘       └──────────────────┘
```

For services that can't reach localhost (mobile apps, desktop apps, hosted
services), SDKs send to the public relay. The local faultline instance runs a
**relay poller** that pulls envelopes every 30 seconds and processes them
through the normal ingest pipeline.

**DSN:** `https://KEY@faultline-relay.fly.dev/PROJECT_ID`

**How the poller works:**
1. Faultline polls `GET /relay/poll?since=lastID&limit=100`
2. Processes each envelope through the local ingest pipeline
3. Acknowledges with `POST /relay/ack`

The relay stores envelopes in SQLite with a 7-day TTL. You don't need to
configure the poller — it starts automatically when `FAULTLINE_RELAY_URL` is set.

### When to use which

| Deployment type | DSN target | Example |
|-----------------|-----------|---------|
| **Local** — service on same machine | `localhost:8080` | Go API, Python worker |
| **Docker** — service in container | `host.docker.internal:8080` | Dockerized web app |
| **Remote** — mobile/desktop app | `faultline-relay.fly.dev` | iOS app, Electron app |
| **Hosted** — cloud service | Either (relay if no tunnel) | Fly.dev, Railway service |

Set the deployment type in project settings to help faultline know what to
expect for health checks.

## Project Settings

Configure per-project metadata at `/dashboard/projects/{id}/settings`.

| Field | Description | Example |
|-------|-------------|---------|
| **Description** | Short project label (<40 chars) | "Gas Town coordinator" |
| **URL** | Service URL (auto-set by heartbeat) | `http://localhost:8000` |
| **Deployment type** | `local`, `remote`, or `hosted` | `local` |
| **Components** | Comma-separated service parts | `web, api, database` |
| **Environments** | Comma-separated env names | `staging, production` |

The URL field is automatically populated when your SDK sends a heartbeat with
a `"url"` in the body (see heartbeat section above). You can also set it
manually here.

Deployment type determines dashboard behavior:
- **local** — faultline can health-check the service directly
- **remote** — reports via relay; no direct health check
- **hosted** — cloud-hosted; health check depends on accessibility

## Framework-Specific Notes

### Next.js

- Use `@sentry/nextjs` (not `@sentry/node`)
- Client-side DSN needs `NEXT_PUBLIC_` prefix: `NEXT_PUBLIC_FAULTLINE_DSN`
- Server-side uses `FAULTLINE_DSN`
- Config files (`sentry.client.config.ts`, `sentry.server.config.ts`) must be
  at the **project root**, not in `app/` or `lib/`

### FastAPI

- `sentry-sdk[fastapi]` auto-captures all unhandled endpoint exceptions
- No per-route setup needed
- Also catches Pydantic validation errors

### Flask

- `sentry-sdk[flask]` auto-captures unhandled exceptions
- Works with Flask error handlers

### Celery / Background Workers

- Workers need **their own** `sentry_sdk.init()` — the FastAPI init only covers
  the web process
- Install: `pip install sentry-sdk[celery]`
- Initialize in the Celery app module, not just the web app

### Docker

- Set `FAULTLINE_DSN` in `docker-compose.yml` environment section or `.env` file
- Use `host.docker.internal` for the host (see Docker section above)
- Rebuild the image if the SDK was added to requirements/package.json

### Sensitive Data (PHI, PII)

Faultline scrubs PII server-side (`FAULTLINE_SCRUB_PII=true` by default).
For additional client-side scrubbing:

```python
def before_send(event, hint):
    # Scrub sensitive fields
    if "request" in event:
        event["request"].pop("data", None)
        event["request"].pop("cookies", None)
    return event

sentry_sdk.init(dsn=..., before_send=before_send)
```

## What happens after integration

1. **Errors detected** → grouped by fingerprint into issue groups
2. **Threshold met** (3 events in 5 min, or 1 fatal) → bead filed in your rig
3. **Slow burn** (any error older than 1 hour with no bead) → bead filed
4. **Polecat auto-dispatched** → `gt sling` spawns a polecat to fix it
5. **Fix merged** → bead closed → quiet period (10 min) → issue resolved
6. **Regression** → error recurs within 24h → new bead + polecat
7. **Repeated regression** (2+) → escalated to Mayor/Overseer

## Dashboard

- **Projects page** (`/dashboard/`) — all projects with status (green/yellow/red), auto-refreshes every 60s
- **Issue list** — sorted by severity, filterable (All/Active/Stabilized)
- **Issue detail** — problem description, resolution explanation, lifecycle timeline, events
- **Live stream** (`/dashboard/live`) — real-time SSE feed across all projects

## Troubleshooting

**"Registered" but not "Running"**: SDK code is pushed but service hasn't been
restarted. Restart the service and ensure FAULTLINE_DSN is set in the runtime
environment.

**Events not appearing**: Check that the DSN is correct and the service can
reach the faultline server (or relay). Test with:
```bash
curl -X POST http://localhost:8080/api/PROJECT_ID/heartbeat \
  -H "X-Sentry-Auth: Sentry sentry_key=YOUR_KEY"
```

**"Unknown event" errors**: You have `traces_sample_rate > 0`. Set it to 0.

**SENTRY_DSN vs FAULTLINE_DSN**: Both work. Use FAULTLINE_DSN to avoid
confusion with external Sentry instances. If migrating from Sentry, just
change the DSN value — the SDK doesn't care about the env var name.
