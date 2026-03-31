package daemon

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

const (
	defaultQuotaDogInterval = 5 * time.Minute
	// quotaDogTimeout is the maximum time allowed for a single rotation cycle.
	quotaDogTimeout = 2 * time.Minute
)

// QuotaDogConfig holds configuration for the quota_dog patrol.
type QuotaDogConfig struct {
	// Enabled controls whether the quota dog runs.
	Enabled bool `json:"enabled"`

	// IntervalStr is how often to run, as a string (e.g., "5m").
	IntervalStr string `json:"interval,omitempty"`
}

// quotaDogInterval returns the configured interval, or the default (5m).
func quotaDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.QuotaDog != nil {
		if config.Patrols.QuotaDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.QuotaDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultQuotaDogInterval
}

// runQuotaDog executes a quota rotation cycle by shelling out to `gt quota rotate`.
// The daemon is a thin ticker — `gt quota rotate` handles scanning for rate-limited
// sessions, planning account assignments, and executing keychain swaps + session restarts.
//
// This follows the daemon's "dumb scheduler" principle: the daemon schedules,
// existing commands do the work. No LLM or molecule needed — pure mechanical rotation.
func (d *Daemon) runQuotaDog() {
	if !d.isPatrolActive("quota_dog") {
		return
	}

	d.logger.Printf("quota_dog: starting rotation cycle")

	ctx, cancel := context.WithTimeout(d.ctx, quotaDogTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, d.gtPath, "quota", "rotate", "--json") //nolint:gosec // G204: gtPath resolved at daemon init
	cmd.Dir = d.config.TownRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Non-fatal: rotation failure shouldn't crash the daemon.
		// Common expected failures: <2 accounts, no rate-limited sessions.
		stderrStr := stderr.String()
		if stderrStr != "" {
			d.logger.Printf("quota_dog: rotation failed (non-fatal): %v: %s", err, stderrStr)
		} else {
			d.logger.Printf("quota_dog: rotation failed (non-fatal): %v", err)
		}
		return
	}

	outStr := stdout.String()
	if outStr != "" && outStr != "[]\n" && outStr != "[]" {
		d.logger.Printf("quota_dog: rotation result: %s", outStr)
	} else {
		d.logger.Printf("quota_dog: no rate-limited sessions detected")
	}
}
