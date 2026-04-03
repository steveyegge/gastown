# Data Model Design

## Summary

Faultline's data model is built on Dolt (MySQL-wire-compatible, git-versioned database) and currently spans ~25 tables covering error tracking, accounts/auth, monitoring, alerting, and Gas Town integration. The schema has grown organically through incremental ALTER TABLE migrations applied in Go code at startup, with no formal migration framework or versioning beyond a single `_schema_version` integer.

The model is sound for its current scale (100 ev/s ceiling, single-server deployment) but has several structural issues that will become increasingly painful: no foreign key constraints, inconsistent ID strategies, schema evolution via scattered Go functions, and growing column bloat on the `issue_groups` table. This analysis proposes targeted improvements for the next development phase.

## Analysis

### Key Considerations

- **Storage engine is Dolt, not MySQL.** Dolt uses a Prolly tree storage engine, not InnoDB. Foreign keys, transactions, and indexing behave differently. Dolt commits are expensive (~5-10ms each), hence the 60-second batch commit strategy (ADR-2).
- **Single shared database.** Faultline shares the Dolt server (port 3307) with Gas Town's beads system. Table names must avoid collisions (hence `ft_events` instead of `events`).
- **No formal migration framework.** Migrations are Go functions (`migrate`, `migrateAccounts`, etc.) called sequentially at startup. There's a `_schema_version` table but only the core migration checks it — all other `migrate*` functions use `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE` with silent error swallowing.
- **Target volume:** 100 ev/s sustained → ~8.6M events/day → ~260M events/month at ceiling. The `ft_events` table is by far the hottest table.
- **Dolt time-travel is a first-class feature.** The batch commit strategy means any query can `AS OF` a past commit to see historical state. This is a unique capability worth preserving in schema decisions.

### Current Schema Inventory

#### Core Error Tracking (hot path)
| Table | Purpose | Growth Rate | Key Access Pattern |
|-------|---------|-------------|-------------------|
| `ft_events` | Raw error/transaction events | ~8.6M/day at ceiling | Insert-heavy, read by group_id, purged at 90 days |
| `issue_groups` | One per unique fingerprint per project | Slow (new errors only) | Read-heavy (dashboard listing), updated on each event |
| `sessions` | Sentry SDK session tracking | Moderate | Insert + update, purged at 90 days |
| `releases` | Aggregated release data | Slow | Upsert on event ingest |
| `beads` | Gas Town bead tracking per group | Slow | Lookup by group_id |

#### Auth & Access Control
| Table | Purpose |
|-------|---------|
| `accounts` | User accounts (email/password) |
| `auth_sessions` | Dashboard login tokens |
| `api_tokens` | Scoped API tokens |
| `teams` / `team_members` / `team_projects` | Team-based project access |

#### Dashboard & Collaboration
| Table | Purpose |
|-------|---------|
| `issue_assignments` | Issue → user assignment |
| `issue_comments` | Comments on issues |
| `notifications` | In-app notification inbox |
| `fingerprint_rules` | Custom fingerprint overrides |
| `ft_error_lifecycle` | Audit log of issue state changes |

#### Alerting
| Table | Purpose |
|-------|---------|
| `alert_rules` | Configurable alert conditions |
| `alert_history` | Fired alert records |

#### Monitoring
| Table | Purpose |
|-------|---------|
| `monitored_databases` / `db_checks` / `db_monitor_state` | Database health monitoring |
| `monitored_containers` / `container_checks` / `container_monitor_state` | Docker container monitoring |
| `health_checks` | Project endpoint health checks |
| `ci_runs` | CI/CD run tracking |

#### Integration
| Table | Purpose |
|-------|---------|
| `integrations_config` | Generic integration configuration |
| `slack_user_mappings` | Slack ↔ faultline account links |

### Options Explored

#### Option 1: Keep Current Approach (Incremental ALTER TABLE)
- **Description**: Continue adding columns via Go `migrate*` functions with `IF NOT EXISTS` guards and silent error swallowing.
- **Pros**: Zero operational overhead, works today, no new dependencies.
- **Cons**: No rollback capability. Impossible to tell which columns exist on a given instance without inspecting the database. Migration ordering is implicit (function call order in `Open()`). Silent error swallowing hides real failures. `issue_groups` already has 20+ columns and growing.
- **Effort**: Low (status quo)

#### Option 2: Numbered SQL Migration Files
- **Description**: Move to a `migrations/` directory with numbered `.sql` files (e.g., `001_initial.sql`, `002_add_accounts.sql`). Track applied migrations in a `_migrations` table. Apply at startup, fail loudly on errors.
- **Pros**: Clear history of schema changes. Rollback possible (down migrations). New contributors can read the schema evolution. Standard pattern (goose, golang-migrate, atlas).
- **Cons**: Requires squashing 25+ existing `migrate*` functions into a baseline. Dolt doesn't support all DDL that MySQL does (no `RENAME COLUMN` in some versions). Migration tools may have Dolt incompatibilities.
- **Effort**: Medium (one-time migration consolidation + ongoing discipline)

#### Option 3: Normalize `issue_groups` Into Separate Tables
- **Description**: Extract the resolution, snooze, and merge tracking columns from `issue_groups` into dedicated tables (`issue_resolutions`, `issue_snoozes`, `issue_merges`).
- **Pros**: `issue_groups` stays focused on error identity. Each concern has its own lifecycle. Queries that don't need resolution data don't pay for those columns.
- **Cons**: More JOINs in dashboard queries. Dolt JOINs are slower than MySQL JOINs. The current column count (20+) isn't actually a performance problem at the target volume.
- **Effort**: Medium-High (requires touching all query functions + dashboard templates)

#### Option 4: Add Foreign Key Constraints
- **Description**: Add FK constraints between tables (e.g., `ft_events.project_id → projects.id`, `issue_assignments.group_id → issue_groups.id`).
- **Pros**: Referential integrity enforced at the DB level. Prevents orphan data. Self-documenting relationships.
- **Cons**: Dolt FK support is functional but has edge cases. INSERT performance degrades with FK checks. The current codebase already maintains referential integrity in Go code. Adds complexity to test setup (must insert parent rows first).
- **Effort**: Medium

### Recommendation

**Short-term (next phase):**

1. **Adopt numbered SQL migrations (Option 2).** This is the highest-impact, lowest-risk improvement. Squash the current 20+ `migrate*` functions into a single baseline `001_baseline.sql`, then use numbered files going forward. Use a minimal custom runner (not a heavy framework) — Dolt compatibility with goose/golang-migrate is uncertain. The runner needs: read `_migrations` table, apply un-applied `.sql` files in order, record each, fail loudly.

2. **Do NOT normalize `issue_groups` yet (skip Option 3).** The column count is manageable and the performance is fine at target volume. Normalization would be a high-churn refactor with marginal benefit. Revisit if the table exceeds ~30 columns or if specific queries show measurable degradation.

3. **Do NOT add foreign keys (skip Option 4).** The Go code already maintains referential integrity. FKs would add Dolt-specific risk for minimal gain. The retention worker already handles orphan cleanup.

**Medium-term improvements:**

4. **Add a `tags` table** for event tags/contexts. Currently tags are buried inside `raw_json` and require JSON extraction for filtering. A dedicated `event_tags (event_id, key, value)` table would enable efficient tag-based queries without parsing JSON. This is the most common feature request pattern in Sentry-like systems.

5. **Add indexes for the environment/release subquery pattern.** The current `ListIssueGroups` uses correlated subqueries (`id IN (SELECT DISTINCT group_id FROM ft_events WHERE ...)`) for environment and release filtering. A composite index on `ft_events(project_id, environment, group_id)` and `ft_events(project_id, release_name, group_id)` would significantly improve these queries.

## Data Lifecycle Analysis

### What needs to persist vs be computed?

| Data | Persist? | Rationale |
|------|----------|-----------|
| Raw events (`ft_events`) | Yes, with TTL (90 days) | Source of truth for debugging. Purged by retention worker. |
| Issue groups | Yes, indefinitely | Aggregated identity. Small table, no purge needed. |
| Event counts / first_seen / last_seen | Persist (denormalized) | Recomputing from events is expensive. Updated atomically on ingest. |
| Hourly event sparklines | Compute on read | Derived from `ft_events` via `HourlyEventCounts()`. Caching could help if slow. |
| Severity magnitude labels | Compute on read | Derived from `event_count` thresholds. No storage needed. |
| Crash-free rate | Persist (on `releases`) | Updated on session close. Too expensive to recompute from all sessions. |

### How will the data grow over time?

At the 100 ev/s design ceiling:
- **`ft_events`**: ~8.6M rows/day, ~260M/month. Purged at 90 days → steady state ~780M rows. This is the dominant storage cost. Each row includes a `raw_json` column (typically 2-10KB) → ~1.5-7.5 TB at steady state. **This is the biggest risk.** The retention worker must be reliable.
- **`issue_groups`**: Grows slowly (new unique fingerprints only). Typical project might have 100-10,000 groups. Never purged.
- **`sessions`**: Moderate growth, purged at 90 days. Depends on SDK configuration.
- **`db_checks` / `container_checks`**: One row per check interval per monitored resource. No retention policy currently configured — **this is a gap** that will cause unbounded growth.
- **Everything else**: Small tables, low growth.

### Access Patterns

| Pattern | Tables | Frequency | Current Performance |
|---------|--------|-----------|-------------------|
| Ingest (write) | `ft_events`, `issue_groups`, `sessions`, `releases` | 100/s ceiling | Good (async, batched commits) |
| Dashboard list | `issue_groups` + subqueries on `ft_events`, `issue_assignments` | Every page load | Adequate, but correlated subqueries will degrade with event volume |
| Issue detail | `issue_groups`, `ft_events` (by group_id), `beads` | Per click | Good (indexed) |
| Event detail | `ft_events` (by PK) | Per click | Good |
| Time-travel | Any table via `AS OF` | Rare (diagnostic) | Dolt-native, works well |
| Retention purge | `ft_events`, `sessions`, `auth_sessions`, `health_checks`, `ci_runs` | Hourly | Adequate at current scale |

### Schema Evolution Strategy

**Current approach (migrate* functions) is fragile because:**
- No way to know what migrations have been applied beyond `_schema_version = 3`
- ALTER TABLE errors are silently swallowed (the column might already exist, or the ALTER might have failed for a different reason)
- No rollback path
- Migration functions are scattered across 20+ files

**Recommended approach:**
1. Squash all existing schema into `001_baseline.sql`
2. Create a `_migrations` table: `(id INT AUTO_INCREMENT, name VARCHAR(256), applied_at DATETIME(6))`
3. At startup: read `_migrations`, apply any un-applied `.sql` files from `internal/db/migrations/` in sorted order
4. Each migration is a single `.sql` file with forward-only DDL (no down migrations — Dolt's time-travel provides the safety net)
5. Remove all `migrate*` functions from Go code except the migration runner itself

## Constraints Identified

1. **Dolt is not MySQL.** Some DDL operations behave differently (e.g., `RENAME COLUMN` support varies by version, FK enforcement has edge cases, `INFORMATION_SCHEMA` queries may be slow). Any migration framework must be tested against Dolt specifically.

2. **Shared database server.** The faultline database shares port 3307 with beads databases. Schema changes must not affect beads stability. Table name collisions are managed by convention (`ft_` prefix).

3. **No down migrations.** Dolt's time-travel (`AS OF`) provides point-in-time recovery, making down migrations unnecessary but also meaning destructive schema changes (DROP COLUMN, DROP TABLE) are permanent after the next Dolt garbage collection.

4. **Event volume drives everything.** At the 100 ev/s ceiling, `ft_events` dominates storage, write load, and query cost. Any schema change to this table must be evaluated for ingest throughput impact.

5. **JSON column dependency.** `raw_json` on `ft_events` stores the complete Sentry event payload. Several features (tag filtering, breadcrumb display, context panels) require parsing this JSON. Extracting frequently-queried fields into dedicated columns or tables is the primary scaling lever.

## Open Questions

1. **Should monitoring check tables (`db_checks`, `container_checks`) have retention policies?** They currently grow unbounded. A 7-day or 30-day TTL (similar to `health_checks`) would prevent accumulation.

2. **Is the `raw_json` column on `ft_events` the right long-term storage strategy?** At scale, storing 2-10KB JSON blobs in Dolt rows is expensive. Alternatives: store in object storage (S3/local files) with a reference in Dolt, or extract common fields and store only the delta. This is a P5+ concern but worth flagging.

3. **Should the `beads` table track more resolution metadata?** Currently it stores `group_id`, `bead_id`, `rig`, `filed_at`, `resolved_at`, `commit_sha`, `merge_ref`, `ci_verified_at`. The `issue_groups` table also stores `root_cause`, `fix_explanation`, `fix_commit`. This split feels accidental — should all resolution data live in one place?

4. **Multi-tenancy model.** There's no `organization_id` column on most tables — the design brief mentions organizations but the schema uses `project_id` as the primary tenant boundary. If organizations are added, should they be a layer above projects (org → projects → data) or remain flat?

## Integration Points

- **Ingest pipeline** (`internal/ingest/`): Writes to `ft_events`, `issue_groups`, `sessions`, `releases`. Any schema change to these tables requires updating the ingest code.
- **Dashboard** (`internal/dashboard/`): Reads from all tables via `internal/db/queries.go`. Query changes propagate to templ templates.
- **Gas Town integration** (`internal/gastown/`): Reads/writes `beads` table, reads `issue_groups`. The `ft_error_lifecycle` audit log tracks all state transitions.
- **Retention worker** (`internal/db/retention.go`): Purges time-series data. Must be updated when adding new time-series tables.
- **Dolt committer** (`internal/db/committer.go`): Batch commits every 60s. All write paths must call `db.MarkDirty()` to trigger commits.
- **Alerting** (`internal/db/alert_rules.go`): Reads `issue_groups` event counts and fires alerts. Depends on `event_count` being accurately maintained.
