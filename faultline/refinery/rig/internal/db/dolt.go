package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DB wraps a sql.DB connected to Dolt.
type DB struct {
	*sql.DB
	dirty atomic.Int64 // incremented on every successful write
}

// MarkDirty records that a write occurred, causing the next committer tick
// to stage and commit.
func (d *DB) MarkDirty() { d.dirty.Add(1) }

// Open connects to Dolt and ensures the schema exists.
func Open(dsn string) (*DB, error) {
	// Ensure parseTime=true so DATETIME columns scan into time.Time.
	if !strings.Contains(dsn, "parseTime") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "parseTime=true"
	}

	// Auto-create the database if it doesn't exist.
	// Parse the database name from the DSN and connect without it first.
	if dbName := extractDBName(dsn); dbName != "" {
		noDB := strings.Replace(dsn, "/"+dbName, "/", 1)
		bootstrap, err := sql.Open("mysql", noDB)
		if err == nil {
			_, _ = bootstrap.Exec("CREATE DATABASE IF NOT EXISTS `" + dbName + "`")
			_ = bootstrap.Close()
		}
	}

	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("dolt open: %w", err)
	}
	sqldb.SetMaxOpenConns(25)
	sqldb.SetMaxIdleConns(10)
	sqldb.SetConnMaxLifetime(5 * time.Minute)
	sqldb.SetConnMaxIdleTime(1 * time.Minute)

	if err := sqldb.Ping(); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt ping: %w", err)
	}
	d := &DB{DB: sqldb}
	if err := d.migrate(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate: %w", err)
	}
	if err := d.migrateGastown(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate gastown: %w", err)
	}
	if err := d.migrateAccounts(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate accounts: %w", err)
	}
	if err := d.migrateAPITokens(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate api_tokens: %w", err)
	}
	if err := d.migrateFingerprintRules(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate fingerprint_rules: %w", err)
	}
	if err := d.migrateAuditLog(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate audit_log: %w", err)
	}
	if err := d.migrateProjectConfig(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate project_config: %w", err)
	}
	if err := d.migrateCIRuns(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate ci_runs: %w", err)
	}
	if err := d.migrateBeadsVerification(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate beads_verification: %w", err)
	}
	if err := d.migrateHealthChecks(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate health_checks: %w", err)
	}
	if err := d.migrateIssueResolution(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate issue_resolution: %w", err)
	}
	if err := d.migrateIssuePlatform(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate issue_platform: %w", err)
	}
	if err := d.migrateAlertRules(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate alert_rules: %w", err)
	}
	if err := d.migrateTeams(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate teams: %w", err)
	}
	if err := d.migrateAssignments(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate assignments: %w", err)
	}
	if err := d.migrateComments(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate comments: %w", err)
	}
	if err := d.migrateNotifications(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate notifications: %w", err)
	}
	if err := d.migrateIntegrations(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate integrations: %w", err)
	}
	if err := d.migrateMergedInto(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate merged_into: %w", err)
	}
	if err := d.migrateSlackUserMappings(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate slack_user_mappings: %w", err)
	}
	if err := d.migrateSnooze(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate snooze: %w", err)
	}
	if err := d.migrateDBMonitors(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate db_monitors: %w", err)
	}
	if err := d.migrateDockerMonitoring(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("dolt migrate docker_monitoring: %w", err)
	}
	return d, nil
}

const schemaVersion = 3

func (d *DB) migrate(ctx context.Context) error {
	// Create version tracking table.
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _schema_version (version INT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var ver int
	err := d.QueryRowContext(ctx, `SELECT version FROM _schema_version LIMIT 1`).Scan(&ver)
	if err == sql.ErrNoRows {
		ver = 0
	} else if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if ver >= schemaVersion {
		return nil // already up to date
	}

	// Drop old v1 tables if they exist (pre-production, no data to preserve).
	if ver < schemaVersion {
		for _, t := range []string{"events", "issue_groups", "beads"} {
			_, _ = d.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", t)) //nolint:gosec // table names are hardcoded above
		}
	}

	stmts := []string{
		// Projects table — maps Sentry DSN keys to project metadata.
		`CREATE TABLE IF NOT EXISTS projects (
			id             BIGINT AUTO_INCREMENT PRIMARY KEY,
			name           VARCHAR(200) NOT NULL,
			slug           VARCHAR(100) UNIQUE NOT NULL,
			dsn_public_key VARCHAR(32) UNIQUE NOT NULL,
			created_at     DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)
		)`,

		// Issue groups — one per unique fingerprint per project.
		`CREATE TABLE IF NOT EXISTS issue_groups (
			id               VARCHAR(36) PRIMARY KEY,
			project_id       BIGINT NOT NULL,
			fingerprint      VARCHAR(64) NOT NULL,
			title            VARCHAR(512) NOT NULL,
			culprit          VARCHAR(512),
			level            VARCHAR(16),
			platform         VARCHAR(64),
			status           VARCHAR(16) NOT NULL DEFAULT 'unresolved',
			first_seen       DATETIME(6) NOT NULL,
			last_seen        DATETIME(6) NOT NULL,
			event_count      INT NOT NULL DEFAULT 1,
			bead_id          VARCHAR(64),
			resolved_at      DATETIME(6),
			regressed_at     DATETIME(6),
			regression_count INT NOT NULL DEFAULT 0,
			UNIQUE KEY uq_project_fingerprint (project_id, fingerprint),
			INDEX idx_project (project_id),
			INDEX idx_status (status)
		)`,

		// Events — individual error/transaction events from SDKs.
		// Named ft_events to avoid collision with beads `events` table in the shared Dolt database.
		`CREATE TABLE IF NOT EXISTS ft_events (
			id             VARCHAR(36) PRIMARY KEY,
			project_id     BIGINT NOT NULL,
			event_id       VARCHAR(36) NOT NULL,
			fingerprint    VARCHAR(64) NOT NULL,
			group_id       VARCHAR(36) NOT NULL,
			level          VARCHAR(16),
			culprit        VARCHAR(512),
			message        TEXT,
			platform       VARCHAR(64),
			environment    VARCHAR(50),
			release_name   VARCHAR(200),
			exception_type VARCHAR(255),
			raw_json       JSON NOT NULL,
			timestamp      DATETIME(6) NOT NULL,
			received_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_project_event (project_id, event_id),
			INDEX idx_group (group_id),
			INDEX idx_fingerprint (project_id, fingerprint),
			INDEX idx_received_at (received_at)
		)`,

		// Releases — aggregated release data from events.
		`CREATE TABLE IF NOT EXISTS releases (
			project_id      BIGINT NOT NULL,
			version         VARCHAR(200) NOT NULL,
			first_seen      DATETIME(6) NOT NULL,
			last_seen       DATETIME(6) NOT NULL,
			event_count     INT NOT NULL DEFAULT 0,
			session_count   INT NOT NULL DEFAULT 0,
			crash_free_rate DOUBLE NOT NULL DEFAULT 1.0,
			UNIQUE KEY uq_project_version (project_id, version),
			INDEX idx_project_lastseen (project_id, last_seen)
		)`,

		// Sessions — Sentry session tracking.
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id   VARCHAR(36) PRIMARY KEY,
			project_id   BIGINT NOT NULL,
			distinct_id  VARCHAR(512),
			status       VARCHAR(16) NOT NULL DEFAULT 'ok',
			errors       INT NOT NULL DEFAULT 0,
			started      DATETIME(6) NOT NULL,
			duration     DOUBLE,
			release_name VARCHAR(256),
			environment  VARCHAR(64),
			user_agent   VARCHAR(512),
			updated_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_project (project_id),
			INDEX idx_status (status)
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate: %w\nSQL: %s", err, s)
		}
	}

	// Upsert schema version.
	if ver == 0 {
		_, err = d.ExecContext(ctx, `INSERT INTO _schema_version (version) VALUES (?)`, schemaVersion)
	} else {
		_, err = d.ExecContext(ctx, `UPDATE _schema_version SET version = ?`, schemaVersion)
	}
	return err
}

// extractDBName parses the database name from a MySQL DSN like "user@tcp(host:port)/dbname?params".
func extractDBName(dsn string) string {
	// Find the path after the host portion.
	idx := strings.Index(dsn, "/")
	if idx < 0 {
		return ""
	}
	rest := dsn[idx+1:]
	// Strip query params.
	if q := strings.Index(rest, "?"); q >= 0 {
		rest = rest[:q]
	}
	return rest
}
