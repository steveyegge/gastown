// Package uptimemon provides periodic HTTP health checks for monitored projects.
// It pings each project's configured URL and records up/down status + response time.
package uptimemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ProjectProvider returns the list of projects to check.
type ProjectProvider interface {
	ListProjectsForHealthCheck(ctx context.Context) ([]ProjectTarget, error)
	RecordHealthCheck(ctx context.Context, projectID int64, up bool, responseTimeMS int, statusCode int) error
}

// ProjectTarget is a project to health-check.
type ProjectTarget struct {
	ID  int64
	URL string
}

// OnStateChangeFunc is called when a project transitions between up and down.
type OnStateChangeFunc func(projectID int64, up bool, responseMS int, statusCode int)

// Monitor runs periodic health checks against project URLs.
type Monitor struct {
	provider      ProjectProvider
	log           *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	client        *http.Client
	lastState     map[int64]bool     // projectID → was up last check
	OnStateChange OnStateChangeFunc  // called on up→down or down→up transitions
	HostRewrite   string             // if set, rewrite "localhost" in URLs to this host (e.g. "host.docker.internal")
}

// New creates an uptime monitor.
func New(provider ProjectProvider, log *slog.Logger, interval, timeout time.Duration) *Monitor {
	if interval == 0 {
		interval = 60 * time.Second
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Monitor{
		provider:  provider,
		log:       log,
		interval:  interval,
		timeout:   timeout,
		client:    &http.Client{Timeout: timeout},
		lastState: make(map[int64]bool),
	}
}

// Run starts the health check loop, blocking until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.log.Info("uptime monitor started", "interval", m.interval)
	// Run once immediately on startup.
	m.checkAll(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAll(ctx)
		}
	}
}

func (m *Monitor) checkAll(ctx context.Context) {
	targets, err := m.provider.ListProjectsForHealthCheck(ctx)
	if err != nil {
		m.log.Error("uptime: list projects", "err", err)
		return
	}

	for _, t := range targets {
		checkURL := t.URL
		if m.HostRewrite != "" {
			checkURL = strings.Replace(checkURL, "localhost", m.HostRewrite, 1)
			checkURL = strings.Replace(checkURL, "127.0.0.1", m.HostRewrite, 1)
		}
		up, responseMS, statusCode := m.ping(ctx, checkURL)
		if err := m.provider.RecordHealthCheck(ctx, t.ID, up, responseMS, statusCode); err != nil {
			m.log.Error("uptime: record check", "err", err, "project", t.ID)
		}

		// Detect state transitions (up→down or down→up).
		wasUp, known := m.lastState[t.ID]
		m.lastState[t.ID] = up
		if known && wasUp != up {
			if up {
				m.log.Info("project recovered", "project", t.ID, "response_ms", responseMS)
			} else {
				m.log.Warn("project down", "project", t.ID, "status_code", statusCode)
			}
			if m.OnStateChange != nil {
				m.OnStateChange(t.ID, up, responseMS, statusCode)
			}
		}
	}
}

func (m *Monitor) ping(ctx context.Context, url string) (up bool, responseMS int, statusCode int) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, 0, 0
	}
	req.Header.Set("User-Agent", "faultline-uptime/1.0")

	resp, err := m.client.Do(req)
	elapsed := time.Since(start)
	responseMS = int(elapsed.Milliseconds())

	if err != nil {
		return false, responseMS, 0
	}
	_ = resp.Body.Close()
	statusCode = resp.StatusCode
	up = statusCode >= 200 && statusCode < 500
	return up, responseMS, statusCode
}

// DB implementation of ProjectProvider — lives here to avoid circular imports.
// The DB methods are added to internal/db via the health_checks table.

// HealthCheck represents a single health check result.
type HealthCheck struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"project_id"`
	Up         bool      `json:"up"`
	ResponseMS int       `json:"response_ms"`
	StatusCode int       `json:"status_code"`
	CheckedAt  time.Time `json:"checked_at"`
}

// DBProvider wraps a *sql.DB to implement ProjectProvider.
type DBProvider struct {
	DB *sql.DB
}

func (p *DBProvider) ListProjectsForHealthCheck(ctx context.Context) ([]ProjectTarget, error) {
	rows, err := p.DB.QueryContext(ctx,
		`SELECT id, config FROM projects WHERE config IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var targets []ProjectTarget
	for rows.Next() {
		var id int64
		var cfgJSON sql.NullString
		if err := rows.Scan(&id, &cfgJSON); err != nil {
			continue
		}
		if !cfgJSON.Valid || cfgJSON.String == "" {
			continue
		}
		var cfg struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(cfgJSON.String), &cfg) != nil || cfg.URL == "" {
			continue
		}
		// Only check HTTP(S) URLs.
		if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
			continue
		}
		targets = append(targets, ProjectTarget{ID: id, URL: cfg.URL})
	}
	return targets, rows.Err()
}

func (p *DBProvider) RecordHealthCheck(ctx context.Context, projectID int64, up bool, responseMS int, statusCode int) error {
	_, err := p.DB.ExecContext(ctx, `
		INSERT INTO health_checks (project_id, up, response_ms, status_code, checked_at)
		VALUES (?, ?, ?, ?, ?)`,
		projectID, up, responseMS, statusCode, time.Now().UTC(),
	)
	return err
}
