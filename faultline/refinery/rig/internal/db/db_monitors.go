package db

import (
	"context"
	"fmt"
)

func (d *DB) migrateDBMonitors(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS monitored_databases (
			id                  VARCHAR(36) PRIMARY KEY,
			project_id          BIGINT,
			name                VARCHAR(200) NOT NULL,
			db_type             VARCHAR(32) NOT NULL,
			connection_string   VARBINARY(2048) NOT NULL,
			enabled             BOOLEAN DEFAULT true,
			check_interval_secs INT DEFAULT 60,
			thresholds          JSON,
			created_at          DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at          DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_md_project (project_id)
		)`,

		`CREATE TABLE IF NOT EXISTS db_checks (
			id          VARCHAR(36) PRIMARY KEY,
			database_id VARCHAR(36) NOT NULL,
			project_id  BIGINT,
			check_type  VARCHAR(64) NOT NULL,
			status      VARCHAR(16) NOT NULL,
			value       DOUBLE,
			message     TEXT,
			checked_at  DATETIME(6) NOT NULL,
			INDEX idx_dc_database (database_id),
			INDEX idx_dc_checked_at (checked_at)
		)`,

		`CREATE TABLE IF NOT EXISTS db_monitor_state (
			database_id          VARCHAR(36) PRIMARY KEY,
			status               VARCHAR(16) DEFAULT 'healthy',
			last_transition_at   DATETIME(6),
			last_check_at        DATETIME(6),
			consecutive_failures INT DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate db_monitors: %w", err)
		}
	}
	return nil
}
