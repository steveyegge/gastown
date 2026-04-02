# Database Error Monitoring for Faultline

**Date:** 2026-04-02
**Status:** Approved
**Scope:** Built-in database poller with state-transition alerting

## Overview

Add database error monitoring to faultline. The system polls configured databases
on an interval, tracks health state (healthy/degraded/down), and fires state
transitions into the existing issue/alert pipeline. Covers both user-configured
databases and faultline's own Dolt instance.

## Supported Databases

| Database   | Monitoring type      | Checks                                                        |
|------------|---------------------|---------------------------------------------------------------|
| PostgreSQL | Built-in poller     | Connection, slow queries, connection pool, replication lag, deadlocks, table bloat |
| Dolt       | Built-in poller     | Connection ping, query latency, commit lag, orphan DB count   |
| Redis      | Built-in poller     | PING, memory usage, eviction rate, slowlog, connected clients, replication lag |
| SQLite     | App-side SDK only   | Errors flow through existing Sentry SDK pipeline              |

MongoDB and MySQL are out of scope for v1.

## Architecture

### Pattern

Same shape as `internal/uptimemon/` — poll targets on interval, track state,
fire callbacks on state transitions. No new binaries; the poller runs inside the
faultline server process.

### Data Model

Three new tables:

**`monitored_databases`** — configuration per database target

```sql
CREATE TABLE IF NOT EXISTS monitored_databases (
  id                  VARCHAR(36) PRIMARY KEY,
  project_id          BIGINT,                        -- NULL for system-level (faultline self-monitoring)
  name                VARCHAR(200) NOT NULL,
  db_type             VARCHAR(32) NOT NULL,          -- 'postgres', 'dolt', 'redis'
  connection_string   VARBINARY(2048) NOT NULL,      -- AES-256-GCM encrypted
  enabled             BOOLEAN DEFAULT true,
  check_interval_secs INT DEFAULT 60,
  thresholds          JSON,                          -- per-check overrides, e.g. {"slow_query_ms": 10000}
  created_at          DATETIME(6) NOT NULL,
  updated_at          DATETIME(6) NOT NULL,
  INDEX idx_project (project_id)
);
```

**`db_checks`** — individual check results

```sql
CREATE TABLE IF NOT EXISTS db_checks (
  id          VARCHAR(36) PRIMARY KEY,
  database_id VARCHAR(36) NOT NULL,
  project_id  BIGINT,
  check_type  VARCHAR(64) NOT NULL,    -- 'connection', 'slow_query', 'replication_lag', 'memory', 'deadlock', etc.
  status      VARCHAR(16) NOT NULL,    -- 'ok', 'warning', 'critical'
  value       DOUBLE,                  -- numeric measurement (ms, bytes, count, etc.)
  message     TEXT,                    -- human-readable detail
  checked_at  DATETIME(6) NOT NULL,
  INDEX idx_database (database_id),
  INDEX idx_checked_at (checked_at)
);
```

**`db_monitor_state`** — persisted state per database

```sql
CREATE TABLE IF NOT EXISTS db_monitor_state (
  database_id          VARCHAR(36) PRIMARY KEY,
  status               VARCHAR(16) DEFAULT 'healthy', -- 'healthy', 'degraded', 'down'
  last_transition_at   DATETIME(6),
  last_check_at        DATETIME(6),
  consecutive_failures INT DEFAULT 0
);
```

### Poller Design (`internal/dbmon/`)

```
dbmon.Monitor
  ├── Run(ctx)              — main loop, wakes every second, dispatches due checks
  ├── checkPostgres(target) — pg_stat_*, connection test, replication
  ├── checkDolt(target)     — ping, query latency, commit lag
  ├── checkRedis(target)    — INFO, SLOWLOG, PING
  └── OnStateChange         — callback for state transitions
```

**Concurrency:** Each database has its own check interval (default 60s). The
monitor wakes every second, checks which databases are due, and dispatches their
checks to a bounded worker pool (10 concurrent checks max). Each check gets a
10s timeout.

**State evaluation:** After all checks complete for a database:
- Any check `critical` → state is `down`
- Any check `warning` → state is `degraded`
- All checks `ok` → state is `healthy`

Compare against `db_monitor_state`. If changed, fire `OnStateChange`.

### Integration with Existing Systems

**Issue creation:** State transitions produce events through the standard pipeline:
- `platform: "database"`
- `level: "error"` (down), `"warning"` (degraded), `"info"` (recovered)
- Fingerprint: `sha256("dbmon|{database_id}|{check_type}")`
- Title: `"{db_type} {db_name}: {check_type} — {message}"`

All existing alert rules, integrations (Slack, PagerDuty, GitHub), and Gas Town
bead filing work automatically.

### Default Thresholds

| Check                | Default          |
|---------------------|------------------|
| Poll interval       | 60s              |
| Slow query          | >5s              |
| Connection usage    | >80% of max      |
| Replication lag     | >10s             |
| Redis memory        | >90% of maxmemory|
| Long-running query  | >30s             |
| Dolt commit lag     | >5min            |
| Check timeout       | 10s              |

### Connection String Security

AES-256-GCM encryption at rest.

- Key from `FAULTLINE_DB_ENCRYPTION_KEY` env var (32-byte hex). If not set,
  auto-generated to `~/.faultline/encryption.key` (mode 0600).
- Encrypted before write, decrypted only in-memory when poller connects.
- API returns masked strings only: `postgres://user:***@host:5432/dbname`.
- No key rotation in v1.

### Faultline Self-Monitoring

Zero-config monitoring of faultline's own Dolt instance:
- Uses the existing internal `*db.DB` connection (no connection string needed)
- `project_id = NULL` (system-level)
- Checks: ping latency, orphan database count, time since last commit
- Wired in `main.go` alongside existing health monitor
- Dedicated system-level dashboard page

### API Endpoints

```
GET    /api/{project_id}/databases           — list monitored databases
POST   /api/{project_id}/databases           — add database
GET    /api/{project_id}/databases/{id}       — get database config
PUT    /api/{project_id}/databases/{id}       — update config
DELETE /api/{project_id}/databases/{id}       — remove database
POST   /api/{project_id}/databases/{id}/test  — test connection
GET    /api/{project_id}/databases/{id}/checks — check history
GET    /api/system/db-health                  — faultline self-monitoring status
```

### Dashboard

**Project-level:**
- "Databases" status cards (healthy/degraded/down per DB)
- Click-through to check history timeline
- Issue groups filtered by `platform: "database"`

**Project settings:**
- "Databases" tab for CRUD
- Connection string input (masked after save)
- DB type selector, interval, threshold overrides
- "Test connection" button

**System-level:**
- Faultline Dolt health page: ping latency, commit lag, orphan count

## Out of Scope (v1)

- MongoDB, MySQL monitoring
- Sidecar agent deployment model
- Encryption key rotation
- Query-level tracing / EXPLAIN analysis
- SQLite server-side monitoring (app-side SDK only)
