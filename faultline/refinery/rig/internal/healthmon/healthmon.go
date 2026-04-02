// Package healthmon provides periodic health monitoring for Dolt and beads infrastructure.
// When issues are detected, they are reported as faultline error events via selfmon.
package healthmon

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// Pinger is the subset of *sql.DB needed by the health monitor.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Config controls the health monitor behavior.
type Config struct {
	// Interval between health checks (default: 60s).
	Interval time.Duration
	// DoltPingTimeout for the DB ping (default: 5s).
	DoltPingTimeout time.Duration
	// RunGTDoctor controls whether `gt doctor` is also executed (default: false).
	// When true, gt doctor runs on every Nth check cycle (DoctorEveryN).
	RunGTDoctor bool
	// DoctorEveryN runs gt doctor every N check cycles (default: 10, i.e. every 10 minutes at 60s interval).
	DoctorEveryN int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Interval:        60 * time.Second,
		DoltPingTimeout: 5 * time.Second,
		RunGTDoctor:     true,
		DoctorEveryN:    10,
	}
}

// OnErrorFunc is called when a health issue is detected.
// The message describes the issue; severity is "warning" or "error".
type OnErrorFunc func(severity, message string)

// Monitor runs periodic health checks against Dolt and beads.
type Monitor struct {
	db      Pinger
	log     *slog.Logger
	cfg     Config
	onError OnErrorFunc
}

// New creates a health monitor.
func New(db Pinger, log *slog.Logger, cfg Config, onError OnErrorFunc) *Monitor {
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.DoltPingTimeout == 0 {
		cfg.DoltPingTimeout = 5 * time.Second
	}
	if cfg.DoctorEveryN == 0 {
		cfg.DoctorEveryN = 10
	}
	return &Monitor{
		db:      db,
		log:     log,
		cfg:     cfg,
		onError: onError,
	}
}

// Run starts the health check loop, blocking until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.log.Info("health monitor started", "interval", m.cfg.Interval, "gt_doctor", m.cfg.RunGTDoctor)
	ticker := time.NewTicker(m.cfg.Interval)
	defer ticker.Stop()

	cycle := 0
	for {
		select {
		case <-ctx.Done():
			m.log.Info("health monitor stopped")
			return
		case <-ticker.C:
			cycle++
			m.checkDolt(ctx)
			if m.cfg.RunGTDoctor && cycle%m.cfg.DoctorEveryN == 0 {
				m.checkDoctor(ctx)
			}
		}
	}
}

// checkDolt pings Dolt to verify connectivity.
func (m *Monitor) checkDolt(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, m.cfg.DoltPingTimeout)
	defer cancel()

	if err := m.db.PingContext(pingCtx); err != nil {
		msg := fmt.Sprintf("Dolt ping failed: %v", err)
		m.log.Error(msg)
		m.onError("error", msg)
	}
}

// checkDoctor runs `gt doctor` and reports any failures.
func (m *Monitor) checkDoctor(ctx context.Context) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "gt", "doctor")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// gt doctor returns non-zero on failures.
		// Extract the summary line for the error report.
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		summary := "gt doctor failed"
		if len(lines) > 0 {
			// Take last few lines which contain the summary.
			start := len(lines) - 3
			if start < 0 {
				start = 0
			}
			summary = fmt.Sprintf("gt doctor failed: %s", strings.Join(lines[start:], " | "))
		}
		m.log.Warn(summary)
		m.onError("warning", summary)
	}
}
