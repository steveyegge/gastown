package daemon

import (
	"time"

	"github.com/steveyegge/gastown/internal/deacon"
)

const (
	// defaultPolecatReaperInterval is the patrol interval for scanning completed
	// polecats. Set to 60s per issue spec — completed polecats are low-cost to
	// scan and quick cleanup keeps the town tidy.
	defaultPolecatReaperInterval = 60 * time.Second
)

// PolecatReaperConfig holds configuration for the polecat_reaper patrol.
// This patrol scans for completed polecats (bead closed, agent not running)
// and reaps them: kills tmux session, removes worktree.
type PolecatReaperConfig struct {
	Enabled          bool   `json:"enabled"`
	DryRun           bool   `json:"dry_run,omitempty"`
	IntervalStr      string `json:"interval,omitempty"`
	IdleThresholdStr string `json:"idle_threshold,omitempty"`
}

// polecatReaperInterval returns the configured interval, or the default (60s).
func polecatReaperInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.PolecatReaper != nil {
		if config.Patrols.PolecatReaper.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.PolecatReaper.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultPolecatReaperInterval
}

// reapCompletedPolecats is the daemon patrol method that delegates to the
// deacon's ScanCompletedPolecats. Called on each ticker fire.
func (d *Daemon) reapCompletedPolecats() {
	if !d.isPatrolActive("polecat_reaper") {
		return
	}

	cfg := &deacon.ReapConfig{
		IdleThreshold: deacon.DefaultIdleThreshold,
	}

	// Apply config overrides
	if d.patrolConfig != nil && d.patrolConfig.Patrols != nil && d.patrolConfig.Patrols.PolecatReaper != nil {
		pc := d.patrolConfig.Patrols.PolecatReaper
		if pc.DryRun {
			cfg.DryRun = true
		}
		if pc.IdleThresholdStr != "" {
			if dur, err := time.ParseDuration(pc.IdleThresholdStr); err == nil && dur > 0 {
				cfg.IdleThreshold = dur
			}
		}
	}

	result, err := deacon.ScanCompletedPolecats(d.config.TownRoot, cfg)
	if err != nil {
		d.logger.Printf("polecat_reaper: scan error: %v", err)
		return
	}

	if result.Reaped > 0 || result.Completed > 0 {
		d.logger.Printf("polecat_reaper: scanned=%d completed=%d reaped=%d",
			result.TotalPolecats, result.Completed, result.Reaped)
	}

	// Log details for reaped polecats
	for _, r := range result.Results {
		if r.Error != "" {
			d.logger.Printf("polecat_reaper: %s/%s: error: %s", r.Rig, r.Polecat, r.Error)
		} else if r.SessionKilled {
			d.logger.Printf("polecat_reaper: %s/%s: reaped (session=%v worktree=%v bead=%s)",
				r.Rig, r.Polecat, r.SessionKilled, r.WorktreeRemoved, r.BeadID)
		}
	}
}
