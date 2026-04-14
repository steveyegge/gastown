package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	// defaultPolecatReaperInterval is the patrol interval for scanning completed
	// polecats. Set to 60s per issue spec — completed polecats are low-cost to
	// scan and quick cleanup keeps the town tidy.
	defaultPolecatReaperInterval = 60 * time.Second

	// polecatReaperDiagEveryN logs a diagnostic summary every N scans even when
	// nothing is reaped, so operators can confirm the patrol is running.
	polecatReaperDiagEveryN = 10
)

// polecatReaperScanCount tracks how many scans have run (for periodic diag logging).
var polecatReaperScanCount atomic.Int64

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
//
// This method includes panic recovery to prevent a crash in the reaper from
// killing the entire daemon process. The daemon's main select loop has no
// recovery wrapper, so an unrecovered panic in any patrol handler terminates
// the daemon — which is exactly the bug this fixes (sbx-gastown-fa1k).
func (d *Daemon) reapCompletedPolecats() {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Printf("polecat_reaper: recovered from panic: %v", r)
		}
	}()

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

	// Use the daemon's context so subprocess calls are bounded by daemon lifecycle.
	result, err := deacon.ScanCompletedPolecatsCtx(d.ctx, d.config.TownRoot, cfg)
	if err != nil {
		d.logger.Printf("polecat_reaper: scan error: %v", err)
		return
	}

	scanNum := polecatReaperScanCount.Add(1)

	// Always log when there's activity (reaped or completed polecats found).
	if result.Reaped > 0 || result.Completed > 0 {
		d.logger.Printf("polecat_reaper: scanned=%d completed=%d reaped=%d",
			result.TotalPolecats, result.Completed, result.Reaped)
	} else if scanNum%polecatReaperDiagEveryN == 1 {
		// Periodic diagnostic: log every Nth scan even when idle so operators
		// can confirm the patrol is running and see what it scanned.
		d.logger.Printf("polecat_reaper: alive (scan #%d, polecats_found=%d)",
			scanNum, result.TotalPolecats)
	}

	// Log decision for each polecat so operators can diagnose classification issues.
	// This is verbose by design — the original bug was silent skip decisions.
	for _, dec := range result.Decisions {
		if dec.Eligible {
			d.logger.Printf("polecat_reaper: %s/%s: eligible (reason=%s bead=%s bead_status=%s)",
				dec.Rig, dec.Polecat, dec.Reason, dec.BeadID, dec.BeadStatus)
		} else {
			d.logger.Printf("polecat_reaper: %s/%s: skip (reason=%s session=%v agent_alive=%v bead=%s bead_status=%s)",
				dec.Rig, dec.Polecat, dec.Reason, dec.HasSession, dec.AgentAlive, dec.BeadID, dec.BeadStatus)
		}
	}

	// Log details for reaped/completed polecats and errors.
	// Trigger archivist extraction for any polecat with a closed bead — whether
	// the reaper killed the session or the polecat self-exited before the scan.
	for _, r := range result.Results {
		if r.Error != "" {
			d.logger.Printf("polecat_reaper: %s/%s: error: %s", r.Rig, r.Polecat, r.Error)
		} else {
			d.logger.Printf("polecat_reaper: %s/%s: completed (session_killed=%v worktree=%v bead=%s)",
				r.Rig, r.Polecat, r.SessionKilled, r.WorktreeRemoved, r.BeadID)

			// Trigger archivist extraction if the polecat had a closed bead.
			if r.BeadID != "" && d.isPatrolActive("archivist") {
				d.spawnArchivistExtraction(r.Rig, r.BeadID)
			}
		}
	}
}

// spawnArchivistExtraction creates an archivist wisp and dispatches an archivist
// to extract knowledge from a completed polecat's bead. The wisp is ephemeral —
// if the extraction is routine, the wisp_reaper cleans it up automatically.
func (d *Daemon) spawnArchivistExtraction(rig, sourceBead string) {
	ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	defer cancel()

	// Create archivist wisp via bd (ephemeral bead)
	desc := fmt.Sprintf("source_bead: %s\nrig: %s\ntrigger_type: post-polecat\nrole: archivist\nformula: mol-archivist-extract", sourceBead, rig)
	createCmd := exec.CommandContext(ctx, "bd", "create",
		"--title", fmt.Sprintf("Archivist: extract from %s", sourceBead),
		"--type", "task",
		"--priority", "3",
		"--labels", rig,
		"--description", desc,
		"--ephemeral",
	)
	createCmd.Dir = d.config.TownRoot
	createCmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(d.config.TownRoot))
	util.SetDetachedProcessGroup(createCmd)

	output, err := createCmd.Output()
	if err != nil {
		d.logger.Printf("archivist: failed to create wisp for %s: %v", sourceBead, err)
		return
	}

	// Parse wisp ID from output (format: "✓ Created issue: <id> — <title>")
	wispID := ""
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "Created") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "issue:" && i+1 < len(parts) {
					wispID = strings.TrimRight(parts[i+1], " —")
					break
				}
			}
		}
	}

	if wispID == "" {
		d.logger.Printf("archivist: created wisp but couldn't parse ID from output: %s", string(output))
		return
	}

	d.logger.Printf("archivist: created extraction wisp %s for %s/%s", wispID, rig, sourceBead)

	// Dispatch archivist via gt sling (non-blocking — let it run in background)
	slingCmd := exec.CommandContext(ctx, "gt", "sling", wispID, rig,
		"--force", "--no-boot", "--formula", "mol-archivist-extract",
		"--var", "source_bead="+sourceBead,
		"--var", "rig="+rig)
	slingCmd.Dir = d.config.TownRoot
	slingCmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(d.config.TownRoot))
	util.SetDetachedProcessGroup(slingCmd)

	if err := slingCmd.Start(); err != nil {
		d.logger.Printf("archivist: failed to dispatch for %s: %v", wispID, err)
		return
	}

	// Don't wait for sling to complete — it spawns a tmux session
	go func() {
		_ = slingCmd.Wait()
	}()

	d.logger.Printf("archivist: dispatched extraction for %s → %s", sourceBead, rig)
}
