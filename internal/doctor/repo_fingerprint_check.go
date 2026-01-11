package doctor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/daemon"
)

// bdDoctorResult represents the JSON output from bd doctor --json.
type bdDoctorResult struct {
	Checks []bdDoctorCheck `json:"checks"`
}

// bdDoctorCheck represents a single check result from bd doctor.
type bdDoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// RepoFingerprintCheck verifies that beads databases have valid repository fingerprints.
// A missing or mismatched fingerprint can cause daemon startup failures and sync issues.
type RepoFingerprintCheck struct {
	FixableCheck
	needsMigration bool   // Cached during Run for use in Fix
	beadsDir       string // Beads directory that needs migration
}

// NewRepoFingerprintCheck creates a new repo fingerprint check.
func NewRepoFingerprintCheck() *RepoFingerprintCheck {
	return &RepoFingerprintCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "repo-fingerprint",
				CheckDescription: "Verify beads database has valid repository fingerprint",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// Run checks if beads databases have valid repo fingerprints.
func (c *RepoFingerprintCheck) Run(ctx *CheckContext) *CheckResult {
	// Reset cached state
	c.needsMigration = false
	c.beadsDir = ""

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); err == nil {
		result := c.checkBeadsDir(filepath.Dir(townBeadsDir), "town")
		if result.Status != StatusOK {
			return result
		}
	}

	// Check rig-level beads if specified
	if ctx.RigName != "" {
		rigBeadsDir := beads.ResolveBeadsDir(ctx.RigPath())
		if _, err := os.Stat(rigBeadsDir); err == nil {
			result := c.checkBeadsDir(filepath.Dir(rigBeadsDir), "rig "+ctx.RigName)
			if result.Status != StatusOK {
				return result
			}
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Repository fingerprints verified",
	}
}

// checkBeadsDir checks a single beads directory for repo fingerprint using bd doctor.
func (c *RepoFingerprintCheck) checkBeadsDir(workDir, location string) *CheckResult {
	// Run bd doctor --json to get fingerprint status
	cmd := exec.Command("bd", "doctor", "--json")
	cmd.Dir = workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// bd doctor exits with non-zero if there are warnings, so ignore exit code
	_ = cmd.Run()

	// Parse JSON output
	var result bdDoctorResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		// If we can't parse bd doctor output, skip this check
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("Skipped %s (bd doctor unavailable)", location),
		}
	}

	// Find the Repo Fingerprint check
	for _, check := range result.Checks {
		if check.Name == "Repo Fingerprint" {
			switch check.Status {
			case "ok":
				return &CheckResult{
					Name:    c.Name(),
					Status:  StatusOK,
					Message: fmt.Sprintf("Fingerprint verified in %s (%s)", location, check.Message),
				}
			case "warning":
				c.needsMigration = true
				c.beadsDir = filepath.Join(workDir, ".beads")
				return &CheckResult{
					Name:    c.Name(),
					Status:  StatusWarning,
					Message: fmt.Sprintf("Fingerprint issue in %s: %s", location, check.Message),
					Details: func() []string {
						if check.Detail != "" {
							return []string{check.Detail}
						}
						return nil
					}(),
					FixHint: "Run 'gt doctor --fix' or 'bd migrate --update-repo-id'",
				}
			case "error":
				c.needsMigration = true
				c.beadsDir = filepath.Join(workDir, ".beads")
				return &CheckResult{
					Name:    c.Name(),
					Status:  StatusError,
					Message: fmt.Sprintf("Fingerprint error in %s: %s", location, check.Message),
					Details: func() []string {
						if check.Detail != "" {
							return []string{check.Detail}
						}
						return nil
					}(),
					FixHint: "Run 'gt doctor --fix' or 'bd migrate --update-repo-id'",
				}
			}
		}
	}

	// Fingerprint check not found in bd doctor output - skip
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("Fingerprint check not applicable for %s", location),
	}
}

// Fix runs bd migrate --update-repo-id and restarts the daemon.
func (c *RepoFingerprintCheck) Fix(ctx *CheckContext) error {
	if !c.needsMigration || c.beadsDir == "" {
		return nil
	}

	// Run bd migrate --update-repo-id
	cmd := exec.Command("bd", "migrate", "--update-repo-id")
	cmd.Dir = filepath.Dir(c.beadsDir) // Parent of .beads directory
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd migrate --update-repo-id failed: %v: %s", err, stderr.String())
	}

	// Restart daemon if running
	running, _, err := daemon.IsRunning(ctx.TownRoot)
	if err == nil && running {
		// Stop daemon
		stopCmd := exec.Command("gt", "daemon", "stop")
		stopCmd.Dir = ctx.TownRoot
		_ = stopCmd.Run() // Ignore errors

		// Wait a moment
		time.Sleep(500 * time.Millisecond)

		// Start daemon
		startCmd := exec.Command("gt", "daemon", "run")
		startCmd.Dir = ctx.TownRoot
		startCmd.Stdin = nil
		startCmd.Stdout = nil
		startCmd.Stderr = nil
		if err := startCmd.Start(); err != nil {
			return fmt.Errorf("failed to restart daemon: %w", err)
		}

		// Wait for daemon to initialize
		time.Sleep(300 * time.Millisecond)
	}

	return nil
}
