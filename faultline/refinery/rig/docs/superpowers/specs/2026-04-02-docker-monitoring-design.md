# Docker Container Monitoring for Faultline

**Date:** 2026-04-02
**Status:** Approved
**Scope:** Docker Compose container health monitoring via Docker socket

## Overview

Monitor Docker containers for health check failures, resource pressure, and
lifecycle events. Containers opt in via labels. State transitions fire into the
existing issue/alert pipeline. Application errors continue to flow through the
SDK — this covers the infrastructure layer the SDK can't see.

## Discovery Model

Label-based discovery via Docker socket. Only containers with
`faultline.monitor=true` are monitored.

```yaml
services:
  my-app:
    labels:
      faultline.monitor: "true"
      faultline.project: "my-app"   # maps to faultline project by slug
```

Unlabeled containers are ignored. If `faultline.project` is omitted, events go
to a system-level "infrastructure" bucket.

## Docker Socket Connection

```
FAULTLINE_DOCKER_SOCKET=/var/run/docker.sock   # default
```

- **Host deployment:** Direct access, needs read permissions on socket.
- **Container deployment:** Mount read-only: `volumes: ["/var/run/docker.sock:/var/run/docker.sock:ro"]`
- If socket unavailable, log warning and retry every 60s. Don't crash faultline.

## Checks and Thresholds

| Check              | Warning                        | Critical                              |
|--------------------|--------------------------------|---------------------------------------|
| Memory usage       | >80% of container limit        | >95% or OOMKilled                     |
| CPU throttling     | >50% of periods throttled      | >80% throttled                        |
| Restart count      | >2 restarts in 5 min           | >5 restarts in 5 min                  |
| Health check       | 1 consecutive failure          | 3 consecutive failures                |
| Container stopped  | —                              | Unexpected stop (exit code != 0)      |
| Disk (volume)      | >85% usage                     | >95% usage                            |

Thresholds are configurable per-project via the dashboard settings page.

## Data Model

### `monitored_containers` — discovered containers

```sql
CREATE TABLE IF NOT EXISTS monitored_containers (
  id              VARCHAR(36) PRIMARY KEY,
  project_id      BIGINT,                          -- NULL for unmapped containers
  container_id    VARCHAR(64) NOT NULL,             -- Docker container ID
  container_name  VARCHAR(200) NOT NULL,
  service_name    VARCHAR(200),                     -- from docker-compose service name
  image           VARCHAR(512),
  enabled         BOOLEAN DEFAULT true,
  thresholds      JSON,                            -- per-container overrides (optional)
  discovered_at   DATETIME(6) NOT NULL,
  last_seen_at    DATETIME(6) NOT NULL,
  INDEX idx_project (project_id),
  INDEX idx_container (container_id)
);
```

### `container_checks` — individual check results

```sql
CREATE TABLE IF NOT EXISTS container_checks (
  id            VARCHAR(36) PRIMARY KEY,
  container_id  VARCHAR(36) NOT NULL,               -- FK to monitored_containers.id
  project_id    BIGINT,
  check_type    VARCHAR(64) NOT NULL,               -- 'health', 'memory', 'cpu', 'restart', 'stopped', 'disk'
  status        VARCHAR(16) NOT NULL,               -- 'ok', 'warning', 'critical'
  value         DOUBLE,                             -- numeric measurement
  message       TEXT,
  checked_at    DATETIME(6) NOT NULL,
  INDEX idx_container (container_id),
  INDEX idx_checked_at (checked_at)
);
```

### `container_monitor_state` — current state per container

```sql
CREATE TABLE IF NOT EXISTS container_monitor_state (
  container_id         VARCHAR(36) PRIMARY KEY,
  status               VARCHAR(16) DEFAULT 'healthy',  -- 'healthy', 'degraded', 'down'
  last_transition_at   DATETIME(6),
  last_check_at        DATETIME(6),
  consecutive_failures INT DEFAULT 0
);
```

### Project-level threshold config

New `docker_thresholds` JSON column on `projects` table, storing per-project
threshold overrides. Editable via dashboard settings "Docker" tab.

## Architecture (`internal/dockermon/`)

Two goroutines:

### Event Watcher

Subscribes to Docker daemon event stream (`/events` API):

- `container.start` with `faultline.monitor=true` → register in `monitored_containers`
- `container.die` with non-zero exit code → immediate critical state transition
- `container.oom` → immediate critical, title: "Container OOMKilled: {service_name}"
- `container.stop` → remove from active monitoring (or mark last_seen)
- `container.health_status: unhealthy` → increment consecutive failures, evaluate threshold

### Stats Poller

Periodic polling via Docker API (`/containers/{id}/stats`):

- Default interval: 30s
- Per container:
  - Memory: `memory_stats.usage / memory_stats.limit` → percentage
  - CPU: `cpu_stats.throttling_data.throttled_periods / total_periods` → throttle ratio
  - Restart count: inspect container, check `RestartCount` delta over window
- Volume disk usage: mount inspection for project volumes

### Monitor Struct

```
dockermon.Monitor
  ├── Run(ctx)                  — starts both goroutines
  ├── watchEvents(ctx)          — Docker event stream subscriber
  ├── pollStats(ctx)            — periodic stats collection
  ├── discoverContainers(ctx)   — initial scan for labeled containers
  ├── evaluateState(container)  — warning/critical threshold logic
  └── OnStateChange             — callback → existing issue pipeline
```

**Startup:** `discoverContainers()` lists all running containers with
`faultline.monitor=true`, populates `monitored_containers`, then starts both
the event watcher and stats poller.

## Integration with Existing Systems

### Issue Creation

State transitions produce events through the standard pipeline:

- `platform: "docker"`
- Fingerprint: `sha256("dockermon|{container_name}|{check_type}")` — uses
  container name (stable across restarts), not container ID (changes on recreate)
- Title examples:
  - "Docker my-app: OOMKilled (exit 137)"
  - "Docker postgres: memory at 92% of limit"
  - "Docker worker: 4 restarts in 5 minutes"
  - "Docker api: health check failing"
- Level: `"error"` (critical/down), `"warning"` (degraded), `"info"` (recovered)

All existing alert rules, integrations (Slack, PagerDuty, GitHub), and Gas Town
bead filing work automatically.

### Dashboard

**Project-level:**
- "Containers" section — status cards per monitored container (healthy/degraded/down)
- Click-through to container detail: memory/CPU timeline charts, event log, current stats
- Issue groups filtered by `platform: "docker"`

**Project settings:**
- "Docker" tab — threshold overrides (editable per-project)
- List of containers currently mapped to this project (read-only, discovered from labels)

**System-level:**
- "Infrastructure" page showing all monitored containers across projects
- Overview: count of healthy/degraded/down

### API Endpoints

```
GET  /api/{project_id}/containers              — list monitored containers
GET  /api/{project_id}/containers/{id}/checks   — check history
GET  /api/{project_id}/containers/{id}/stats    — current stats snapshot
PUT  /api/{project_id}/settings/docker          — update threshold overrides
GET  /api/system/containers                     — all containers (admin)
```

## Dependencies

- `github.com/docker/docker/client` — Docker SDK for Go
- Docker socket access (read-only sufficient)

## Out of Scope (v1)

- Kubernetes support
- Docker Swarm
- Container log tailing (errors go through SDK)
- Network monitoring between containers
- Remote Docker daemon (TCP socket)
- Container image vulnerability scanning
