package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// defaultDiskWatchdogInterval is how often the disk watchdog checks disk usage.
	defaultDiskWatchdogInterval = 5 * time.Minute

	// diskWarnThreshold triggers a P1 warning bead + nudge mayor.
	diskWarnThreshold = 0.80 // 80% full

	// diskCleanupThreshold triggers safe cleanup + P0 bead.
	diskCleanupThreshold = 0.90 // 90% full

	// diskEmergencyThreshold triggers emergency cleanup + mayor escalation.
	diskEmergencyThreshold = 0.95 // 95% full

	// diskWatchdogCooldown prevents repeated alerts for the same threshold level.
	// A new alert fires only after usage drops below threshold or cooldown expires.
	diskWatchdogCooldown = 30 * time.Minute
)

// DiskWatchdogConfig holds configuration for the disk_watchdog patrol.
type DiskWatchdogConfig struct {
	// Enabled controls whether the disk watchdog runs.
	Enabled bool `json:"enabled"`

	// IntervalStr is how often to check, as a string (e.g., "5m"). Default: "5m".
	IntervalStr string `json:"interval,omitempty"`

	// WarnThreshold is the fraction of disk full that triggers a P1 warning.
	// Default: 0.80 (80%).
	WarnThreshold *float64 `json:"warn_threshold,omitempty"`

	// CleanupThreshold is the fraction of disk full that triggers safe cleanup.
	// Default: 0.90 (90%).
	CleanupThreshold *float64 `json:"cleanup_threshold,omitempty"`

	// EmergencyThreshold is the fraction that triggers emergency cleanup + escalation.
	// Default: 0.95 (95%).
	EmergencyThreshold *float64 `json:"emergency_threshold,omitempty"`
}

// diskWatchdogInterval returns the configured interval or the default.
func diskWatchdogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.DiskWatchdog != nil {
		if config.Patrols.DiskWatchdog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.DiskWatchdog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultDiskWatchdogInterval
}

// diskWatchdogThresholds returns the effective thresholds from config or defaults.
func diskWatchdogThresholds(config *DaemonPatrolConfig) (warn, cleanup, emergency float64) {
	warn = diskWarnThreshold
	cleanup = diskCleanupThreshold
	emergency = diskEmergencyThreshold

	if config != nil && config.Patrols != nil && config.Patrols.DiskWatchdog != nil {
		cfg := config.Patrols.DiskWatchdog
		if cfg.WarnThreshold != nil && *cfg.WarnThreshold > 0 {
			warn = *cfg.WarnThreshold
		}
		if cfg.CleanupThreshold != nil && *cfg.CleanupThreshold > 0 {
			cleanup = *cfg.CleanupThreshold
		}
		if cfg.EmergencyThreshold != nil && *cfg.EmergencyThreshold > 0 {
			emergency = *cfg.EmergencyThreshold
		}
	}
	return
}

// DiskUsageInfo holds disk usage information for the root filesystem.
type DiskUsageInfo struct {
	// TotalBytes is the total disk capacity.
	TotalBytes uint64

	// FreeBytes is the number of free bytes available to non-root users.
	FreeBytes uint64

	// UsedFraction is the fraction of disk used (0.0–1.0).
	UsedFraction float64
}

// diskWatchdogState tracks the last alert level to implement cooldown.
type diskWatchdogState struct {
	lastLevel    string    // "warn", "cleanup", "emergency", or ""
	lastAlertAt  time.Time // when the last alert was sent
}

// runDiskWatchdog checks free disk space and takes action if thresholds are exceeded.
func (d *Daemon) runDiskWatchdog() {
	if !d.isPatrolActive("disk_watchdog") {
		return
	}

	info, err := getDiskUsage(d.config.TownRoot)
	if err != nil {
		d.logger.Printf("disk_watchdog: failed to get disk usage: %v", err)
		return
	}

	d.logger.Printf("disk_watchdog: disk usage %.1f%% (free: %s, total: %s)",
		info.UsedFraction*100,
		formatBytes(info.FreeBytes),
		formatBytes(info.TotalBytes),
	)

	warnThreshold, cleanupThreshold, emergencyThreshold := diskWatchdogThresholds(d.patrolConfig)

	switch {
	case info.UsedFraction >= emergencyThreshold:
		d.handleDiskEmergency(info)
	case info.UsedFraction >= cleanupThreshold:
		d.handleDiskCleanup(info)
	case info.UsedFraction >= warnThreshold:
		d.handleDiskWarning(info)
	default:
		// Below warning threshold — reset state
		d.diskWatchdogState = diskWatchdogState{}
	}
}

// handleDiskWarning creates a P1 bead and nudges mayor when disk is 80-90% full.
func (d *Daemon) handleDiskWarning(info *DiskUsageInfo) {
	if d.diskWatchdogState.lastLevel == "warn" &&
		time.Since(d.diskWatchdogState.lastAlertAt) < diskWatchdogCooldown {
		d.logger.Printf("disk_watchdog: warn cooldown active (last alert %v ago), skipping",
			time.Since(d.diskWatchdogState.lastAlertAt).Round(time.Minute))
		return
	}

	msg := fmt.Sprintf("Disk usage at %.1f%% — %.1f GB free of %.1f GB total. Run 'gt maintain --disk' to reclaim space.",
		info.UsedFraction*100,
		float64(info.FreeBytes)/(1024*1024*1024),
		float64(info.TotalBytes)/(1024*1024*1024),
	)
	d.logger.Printf("disk_watchdog: WARNING — %s", msg)

	// Create P1 bead for mayor awareness.
	d.createDiskAlertBead("P1", msg, "disk_usage_warning")

	// Nudge mayor to take action.
	d.nudge("mayor/", fmt.Sprintf("DISK WARNING: %s", msg))

	d.diskWatchdogState = diskWatchdogState{lastLevel: "warn", lastAlertAt: time.Now()}
}

// handleDiskCleanup runs safe cleanup and creates a P0 bead when disk is 90-95% full.
func (d *Daemon) handleDiskCleanup(info *DiskUsageInfo) {
	if d.diskWatchdogState.lastLevel == "cleanup" &&
		time.Since(d.diskWatchdogState.lastAlertAt) < diskWatchdogCooldown {
		d.logger.Printf("disk_watchdog: cleanup cooldown active, skipping")
		return
	}

	msg := fmt.Sprintf("Disk usage at %.1f%% — running safe cleanup (log rotation + go clean -cache).",
		info.UsedFraction*100)
	d.logger.Printf("disk_watchdog: CLEANUP — %s", msg)

	// Run gt maintain --disk for safe cleanup.
	d.runDiskMaintain(false)

	// Re-check usage after cleanup.
	afterInfo, err := getDiskUsage(d.config.TownRoot)
	if err == nil {
		msg = fmt.Sprintf("Disk was at %.1f%%, after cleanup at %.1f%% (freed %s). Run 'gt maintain --disk' manually if needed.",
			info.UsedFraction*100,
			afterInfo.UsedFraction*100,
			formatBytes(info.FreeBytes-afterInfo.FreeBytes),
		)
	}

	// Create P0 bead.
	d.createDiskAlertBead("P0", msg, "disk_usage_critical")

	// Nudge mayor.
	d.nudge("mayor/", fmt.Sprintf("DISK CRITICAL (90%%): %s", msg))

	d.diskWatchdogState = diskWatchdogState{lastLevel: "cleanup", lastAlertAt: time.Now()}
}

// handleDiskEmergency runs emergency cleanup and escalates to mayor when disk is 95%+ full.
func (d *Daemon) handleDiskEmergency(info *DiskUsageInfo) {
	if d.diskWatchdogState.lastLevel == "emergency" &&
		time.Since(d.diskWatchdogState.lastAlertAt) < diskWatchdogCooldown {
		d.logger.Printf("disk_watchdog: emergency cooldown active, skipping")
		return
	}

	msg := fmt.Sprintf("DISK EMERGENCY: %.1f%% full — only %s free. Running emergency cleanup.",
		info.UsedFraction*100,
		formatBytes(info.FreeBytes),
	)
	d.logger.Printf("disk_watchdog: EMERGENCY — %s", msg)

	// Run emergency cleanup (more aggressive than normal cleanup).
	d.runDiskMaintain(true)

	// Run git gc --auto on all worktrees.
	d.runGitGCOnWorktrees()

	// Re-check after cleanup.
	afterInfo, err := getDiskUsage(d.config.TownRoot)
	afterMsg := msg
	if err == nil {
		afterMsg = fmt.Sprintf("DISK EMERGENCY resolved from %.1f%% to %.1f%% (freed %s). Monitor closely.",
			info.UsedFraction*100,
			afterInfo.UsedFraction*100,
			formatBytes(info.FreeBytes-afterInfo.FreeBytes),
		)
	}

	// Create P0 bead.
	d.createDiskAlertBead("P0", afterMsg, "disk_usage_emergency")

	// Escalate to mayor.
	d.escalate("disk_watchdog", afterMsg)

	d.diskWatchdogState = diskWatchdogState{lastLevel: "emergency", lastAlertAt: time.Now()}
}

// runDiskMaintain calls gt maintain --disk [--emergency] for cleanup.
func (d *Daemon) runDiskMaintain(emergency bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	args := []string{"maintain", "--disk", "--force"}
	if emergency {
		args = append(args, "--emergency")
	}

	cmd := exec.CommandContext(ctx, d.gtPath, args...)
	cmd.Dir = d.config.TownRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Printf("disk_watchdog: gt maintain --disk failed: %v\nOutput: %s", err, string(output))
	} else {
		d.logger.Printf("disk_watchdog: gt maintain --disk completed")
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if line != "" {
				d.logger.Printf("disk_watchdog:   %s", line)
			}
		}
	}
}

// runGitGCOnWorktrees runs git gc --auto on all known rig worktrees.
func (d *Daemon) runGitGCOnWorktrees() {
	entries, err := os.ReadDir(d.config.TownRoot)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		rigDir := filepath.Join(d.config.TownRoot, entry.Name())
		// Check for polecats subdirectory with worktrees.
		polecatsDir := filepath.Join(rigDir, "polecats")
		if _, err := os.Stat(polecatsDir); err != nil {
			continue
		}
		// Find worktrees under polecats/<lang>/<rig>.
		_ = filepath.Walk(polecatsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			// Check if this dir has a .git (worktree marker).
			gitMarker := filepath.Join(path, ".git")
			if _, err := os.Stat(gitMarker); err != nil {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "git", "gc", "--auto")
			cmd.Dir = path
			if out, err := cmd.CombinedOutput(); err != nil {
				d.logger.Printf("disk_watchdog: git gc --auto in %s failed: %v: %s", path, err, string(out))
			} else {
				d.logger.Printf("disk_watchdog: git gc --auto completed in %s", path)
			}
			return nil
		})
	}
}

// createDiskAlertBead creates a bead to track a disk alert.
func (d *Daemon) createDiskAlertBead(priority, message, beadType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	title := fmt.Sprintf("Disk alert: %s", beadType)
	cmd := exec.CommandContext(ctx, d.bdPath, "create",
		"--title", title,
		"--type", "bug",
		"--priority", priorityToInt(priority),
		"--notes", message,
	)
	cmd.Dir = d.config.TownRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		d.logger.Printf("disk_watchdog: failed to create %s bead: %v: %s", priority, err, string(out))
	} else {
		d.logger.Printf("disk_watchdog: created %s bead for %s", priority, beadType)
	}
}

// nudge sends an ephemeral nudge message to an agent.
func (d *Daemon) nudge(target, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, d.gtPath, "nudge", target, message)
	cmd.Dir = d.config.TownRoot
	if err := cmd.Run(); err != nil {
		d.logger.Printf("disk_watchdog: nudge to %s failed: %v", target, err)
	}
}

// priorityToInt converts a priority label to bd CLI integer.
func priorityToInt(priority string) string {
	switch priority {
	case "P0":
		return "0"
	case "P1":
		return "1"
	case "P2":
		return "2"
	default:
		return "1"
	}
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
