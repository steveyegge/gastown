package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StaleBeadsRedirectCheck detects .beads directories that have both a redirect
// file AND stale data files. This can happen when:
// - A rig is added from a repo that already has .beads/ tracked in git
// - Crew workspaces are cloned from repos with existing .beads/ files
// - SetupRedirect failed or was run before cleanup logic was added
//
// When both redirect and data files exist, bd commands may use stale data
// instead of following the redirect.
type StaleBeadsRedirectCheck struct {
	FixableCheck
	staleLocations []string // Cached for Fix
}

// NewStaleBeadsRedirectCheck creates a new stale beads redirect check.
func NewStaleBeadsRedirectCheck() *StaleBeadsRedirectCheck {
	return &StaleBeadsRedirectCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stale-beads-redirect",
				CheckDescription: "Check for stale files in .beads directories with redirects",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// staleFilePatterns are runtime files that should NOT exist alongside a redirect.
// These are gitignored runtime files that would conflict with redirected data.
// Note: config.yaml is NOT included because it may be tracked in git.
var staleFilePatterns = []string{
	// SQLite databases
	"*.db",
	"*.db-*",
	"*.db?*",
	// JSONL data files (tracked but stale in redirect locations)
	"issues.jsonl",
	"interactions.jsonl",
	// Sync and metadata
	"metadata.json",
	"sync-state.json",
	"last-touched",
	".local_version",
	// Daemon runtime files
	"daemon.lock",
	"daemon.log",
	"daemon.pid",
	"bd.sock",
}

// Run checks for stale files in .beads directories that have redirects.
func (c *StaleBeadsRedirectCheck) Run(ctx *CheckContext) *CheckResult {
	var staleLocations []string

	// Get list of rigs to scan
	rigDirs, err := findRigDirs(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not scan rigs: %v", err),
		}
	}

	// For each rig, check all potential .beads locations
	for _, rigDir := range rigDirs {
		locations := getBeadsDirsToCheck(rigDir)
		for _, beadsDir := range locations {
			if hasRedirectWithStaleFiles(beadsDir) {
				// Make path relative to town root for readability
				relPath, _ := filepath.Rel(ctx.TownRoot, beadsDir)
				if relPath == "" {
					relPath = beadsDir
				}
				staleLocations = append(staleLocations, relPath)
			}
		}
	}

	// Cache for Fix
	c.staleLocations = staleLocations

	if len(staleLocations) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale beads files found in redirect locations",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d location(s) have stale beads files alongside redirect", len(staleLocations)),
		Details: staleLocations,
		FixHint: "Run 'gt doctor --fix' to remove stale files",
	}
}

// Fix removes stale files from .beads directories that have redirects.
func (c *StaleBeadsRedirectCheck) Fix(ctx *CheckContext) error {
	for _, relPath := range c.staleLocations {
		beadsDir := filepath.Join(ctx.TownRoot, relPath)
		if err := cleanStaleBeadsFiles(beadsDir); err != nil {
			return fmt.Errorf("cleaning %s: %w", relPath, err)
		}
	}
	return nil
}

// findRigDirs returns all rig directories in the town.
func findRigDirs(townRoot string) ([]string, error) {
	var rigs []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip hidden dirs, mayor, docs
		if strings.HasPrefix(name, ".") || name == "mayor" || name == "docs" {
			continue
		}

		rigPath := filepath.Join(townRoot, name)

		// A rig should have at least a .git directory (be a git repo)
		// or have a mayor/rig subdirectory
		if isLikelyRig(rigPath) {
			rigs = append(rigs, rigPath)
		}
	}

	return rigs, nil
}

// isLikelyRig checks if a directory looks like a rig.
func isLikelyRig(path string) bool {
	// Check for .git (it's a git repo)
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	// Check for mayor/rig (has the standard rig structure)
	if _, err := os.Stat(filepath.Join(path, "mayor", "rig")); err == nil {
		return true
	}
	// Check for .beads directory (has beads configured)
	if _, err := os.Stat(filepath.Join(path, ".beads")); err == nil {
		return true
	}
	return false
}

// getBeadsDirsToCheck returns all .beads directories to check for a rig.
func getBeadsDirsToCheck(rigDir string) []string {
	var dirs []string

	// Rig root .beads
	rigBeads := filepath.Join(rigDir, ".beads")
	if _, err := os.Stat(rigBeads); err == nil {
		dirs = append(dirs, rigBeads)
	}

	// Crew .beads directories: <rig>/crew/*/.beads
	crewDir := filepath.Join(rigDir, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				beadsDir := filepath.Join(crewDir, entry.Name(), ".beads")
				if _, err := os.Stat(beadsDir); err == nil {
					dirs = append(dirs, beadsDir)
				}
			}
		}
	}

	// Refinery .beads: <rig>/refinery/rig/.beads
	refineryBeads := filepath.Join(rigDir, "refinery", "rig", ".beads")
	if _, err := os.Stat(refineryBeads); err == nil {
		dirs = append(dirs, refineryBeads)
	}

	// Polecats .beads directories: <rig>/polecats/*/.beads
	polecatsDir := filepath.Join(rigDir, "polecats")
	if entries, err := os.ReadDir(polecatsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				beadsDir := filepath.Join(polecatsDir, entry.Name(), ".beads")
				if _, err := os.Stat(beadsDir); err == nil {
					dirs = append(dirs, beadsDir)
				}
			}
		}
	}

	return dirs
}

// hasRedirectWithStaleFiles checks if a .beads directory has both a redirect
// file and stale data files.
func hasRedirectWithStaleFiles(beadsDir string) bool {
	// Must have redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	if _, err := os.Stat(redirectPath); os.IsNotExist(err) {
		return false
	}

	// Check for any stale files
	for _, pattern := range staleFilePatterns {
		matches, err := filepath.Glob(filepath.Join(beadsDir, pattern))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			return true
		}
	}

	return false
}

// cleanStaleBeadsFiles removes stale files from a .beads directory,
// preserving the redirect file and .gitignore.
func cleanStaleBeadsFiles(beadsDir string) error {
	// Verify redirect exists before cleaning
	redirectPath := filepath.Join(beadsDir, "redirect")
	if _, err := os.Stat(redirectPath); os.IsNotExist(err) {
		return fmt.Errorf("no redirect file found - refusing to clean")
	}

	// Remove files matching stale patterns
	for _, pattern := range staleFilePatterns {
		matches, err := filepath.Glob(filepath.Join(beadsDir, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			if err := os.RemoveAll(match); err != nil {
				return fmt.Errorf("removing %s: %w", filepath.Base(match), err)
			}
		}
	}

	// Also remove mq directory if it exists
	mqDir := filepath.Join(beadsDir, "mq")
	if _, err := os.Stat(mqDir); err == nil {
		if err := os.RemoveAll(mqDir); err != nil {
			return fmt.Errorf("removing mq: %w", err)
		}
	}

	return nil
}
