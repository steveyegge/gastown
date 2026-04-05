// Package clouddb provides a numbered SQL migration runner for the
// faultline_cloud Dolt database used in cloud mode.
package clouddb

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB wraps a sql.DB connected to the faultline_cloud Dolt database.
type DB struct {
	*sql.DB
}

// Open connects to the faultline_cloud Dolt database, runs pending migrations,
// and returns the ready connection. The dsn should point at the Dolt server
// (e.g. "root@tcp(127.0.0.1:3307)/faultline_cloud").
func Open(dsn string, log *slog.Logger) (*DB, error) {
	if !strings.Contains(dsn, "parseTime") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "parseTime=true"
	}

	// Auto-create the database if it doesn't exist.
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
		return nil, fmt.Errorf("clouddb open: %w", err)
	}
	sqldb.SetMaxOpenConns(10)
	sqldb.SetMaxIdleConns(5)
	sqldb.SetConnMaxLifetime(5 * time.Minute)

	if err := sqldb.Ping(); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("clouddb ping: %w", err)
	}

	d := &DB{DB: sqldb}
	n, err := d.migrate(context.Background(), log)
	if err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("clouddb migrate: %w", err)
	}
	if n > 0 {
		log.Info("cloud database migrations applied", "count", n)
	}
	return d, nil
}

// migrate applies all pending numbered SQL migrations from the embedded
// migrations/ directory. Each migration is tracked in schema_migrations
// so it runs exactly once.
func (d *DB) migrate(ctx context.Context, log *slog.Logger) (int, error) {
	// Ensure the tracking table exists.
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INT NOT NULL PRIMARY KEY,
		name       VARCHAR(255) NOT NULL,
		applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
	)`)
	if err != nil {
		return 0, fmt.Errorf("create schema_migrations: %w", err)
	}

	// Load applied versions.
	applied, err := d.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}

	// Discover and sort migration files.
	files, err := listMigrations()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, mig := range files {
		if applied[mig.version] {
			continue
		}

		sqlBytes, err := migrationFS.ReadFile("migrations/" + mig.filename)
		if err != nil {
			return count, fmt.Errorf("read migration %s: %w", mig.filename, err)
		}

		log.Info("applying migration", "version", mig.version, "name", mig.name)

		// Execute the migration SQL. Migrations may contain multiple
		// statements separated by semicolons.
		for _, stmt := range splitStatements(string(sqlBytes)) {
			if stmt == "" {
				continue
			}
			if _, err := d.ExecContext(ctx, stmt); err != nil {
				return count, fmt.Errorf("migration %s: %w\nSQL: %s", mig.filename, err, stmt)
			}
		}

		// Record the migration.
		_, err = d.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`,
			mig.version, mig.name)
		if err != nil {
			return count, fmt.Errorf("record migration %s: %w", mig.filename, err)
		}
		count++
	}
	return count, nil
}

// appliedVersions returns a set of already-applied migration version numbers.
func (d *DB) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	m := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		m[v] = true
	}
	return m, rows.Err()
}

type migration struct {
	version  int
	name     string
	filename string
}

// listMigrations reads the embedded migrations/ directory and returns them
// sorted by version number. Filenames must be NNN_name.sql.
func listMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migs []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		ver, name, ok := parseMigrationName(e.Name())
		if !ok {
			continue
		}
		migs = append(migs, migration{version: ver, name: name, filename: e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

// parseMigrationName extracts version and name from "001_projects.sql".
func parseMigrationName(filename string) (int, string, bool) {
	base := strings.TrimSuffix(filename, ".sql")
	idx := strings.Index(base, "_")
	if idx < 1 {
		return 0, "", false
	}
	var ver int
	for _, c := range base[:idx] {
		if c < '0' || c > '9' {
			return 0, "", false
		}
		ver = ver*10 + int(c-'0')
	}
	return ver, base[idx+1:], true
}

// splitStatements splits SQL text on semicolons, trimming whitespace and
// skipping empty results. This handles the common case of multiple DDL
// statements in a single migration file.
func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func extractDBName(dsn string) string {
	idx := strings.Index(dsn, "/")
	if idx < 0 {
		return ""
	}
	rest := dsn[idx+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		rest = rest[:q]
	}
	return rest
}
