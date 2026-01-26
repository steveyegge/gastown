package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/style"
)

// gitFileStatus represents the git status of a file.
type gitFileStatus string

const (
	gitStatusUntracked       gitFileStatus = "untracked"        // File not tracked by git
	gitStatusTrackedClean    gitFileStatus = "tracked-clean"    // Tracked, no local modifications
	gitStatusTrackedModified gitFileStatus = "tracked-modified" // Tracked with local modifications
	gitStatusUnknown         gitFileStatus = "unknown"          // Not in a git repo or error
)

// ClaudeSettingsCheck identifies stale Claude settings files for cleanup.
// Detects settings in wrong locations (parent directories instead of working directories)
// and old settings.json files (should be settings.local.json for gitignore compatibility).
// Agent startup (EnsureSettingsForRole) handles creation of correct settings.
type ClaudeSettingsCheck struct {
	FixableCheck
	staleSettings []staleSettingsInfo
}

type staleSettingsInfo struct {
	path          string        // Full path to settings.json
	agentType     string        // e.g., "witness", "refinery", "deacon", "mayor"
	rigName       string        // Rig name (empty for town-level agents)
	sessionName   string        // tmux session name for cycling
	missing       []string      // What's missing from the settings
	wrongLocation bool          // True if file is in wrong location (should be deleted)
	gitStatus     gitFileStatus // Git status for wrong-location files (for safe deletion)
}

// NewClaudeSettingsCheck creates a new Claude settings validation check.
func NewClaudeSettingsCheck() *ClaudeSettingsCheck {
	return &ClaudeSettingsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "claude-settings",
				CheckDescription: "Clean up stale settings (old settings.json, wrong locations)",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run finds stale settings files that should be cleaned up.
// Stale files include: settings in parent directories, old settings.json files.
// Agent startup (EnsureSettingsForRole) creates correct settings.local.json files.
func (c *ClaudeSettingsCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleSettings = nil

	var details []string
	var hasModifiedFiles bool

	// Find all stale settings files
	staleFiles := c.findStaleSettingsFiles(ctx.TownRoot)

	for _, sf := range staleFiles {
		// Check git status to determine safe deletion strategy
		sf.gitStatus = c.getGitFileStatus(sf.path)
		c.staleSettings = append(c.staleSettings, sf)

		// Provide detailed message based on reason and git status
		var statusMsg string
		if len(sf.missing) > 0 {
			statusMsg = sf.missing[0] // missing contains the reason
		}
		switch sf.gitStatus {
		case gitStatusUntracked:
			statusMsg += ", untracked (safe to delete)"
		case gitStatusTrackedClean:
			statusMsg += ", tracked but unmodified (safe to delete)"
		case gitStatusTrackedModified:
			statusMsg += ", tracked with local modifications (manual review needed)"
			hasModifiedFiles = true
		default:
			statusMsg += " (safe to delete)"
		}
		details = append(details, fmt.Sprintf("%s: %s", sf.path, statusMsg))
	}

	if len(c.staleSettings) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale Claude settings files found",
		}
	}

	fixHint := "Run 'gt doctor --fix' to delete stale settings (agent startup recreates correct ones)"
	if hasModifiedFiles {
		fixHint = "Run 'gt doctor --fix' to delete safe files. Files with local modifications require manual review."
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d stale Claude settings file(s) to clean up", len(c.staleSettings)),
		Details: details,
		FixHint: fixHint,
	}
}

// findStaleSettingsFiles locates stale settings files that should be cleaned up.
// Stale files include:
// - Any settings.json (old filename, should be settings.local.json)
// - Settings in parent directories (should be in working directories)
// - Town root settings/CLAUDE.md (should be in mayor/)
//
// Working directories per role:
// - witness: witness/ or witness/rig/ (witnessDir() decides based on disk state)
// - refinery: refinery/rig/
// - crew: crew/<name>/
// - polecat: polecats/<name>/<rig>/
func (c *ClaudeSettingsCheck) findStaleSettingsFiles(townRoot string) []staleSettingsInfo {
	var files []staleSettingsInfo

	// Settings filenames to check (settings.json is stale, settings.local.json is current)
	settingsFilenames := []string{"settings.json", "settings.local.json"}

	// Check for STALE settings at town root (~/gt/.claude/*)
	// This is WRONG - settings here pollute ALL child workspaces via directory traversal.
	// Mayor settings should be at ~/gt/mayor/.claude/ instead.
	for _, filename := range settingsFilenames {
		staleTownRootSettings := filepath.Join(townRoot, ".claude", filename)
		if fileExists(staleTownRootSettings) {
			files = append(files, staleSettingsInfo{
				path:          staleTownRootSettings,
				agentType:     "mayor",
				sessionName:   "hq-mayor",
				wrongLocation: true,
				missing:       []string{"wrong location (should be in mayor/.claude/)"},
			})
		}
	}

	// Check for STALE CLAUDE.md at town root (~/gt/CLAUDE.md)
	// This is WRONG - CLAUDE.md here is inherited by ALL agents via directory traversal.
	staleTownRootCLAUDEmd := filepath.Join(townRoot, "CLAUDE.md")
	if fileExists(staleTownRootCLAUDEmd) {
		files = append(files, staleSettingsInfo{
			path:          staleTownRootCLAUDEmd,
			agentType:     "mayor",
			sessionName:   "hq-mayor",
			wrongLocation: true,
			missing:       []string{"wrong location (should be in mayor/)"},
		})
	}

	// Town-level agents: mayor and deacon
	// Check for old settings.json (should be settings.local.json)
	for _, agent := range []struct {
		name    string
		session string
	}{
		{"mayor", "hq-mayor"},
		{"deacon", "hq-deacon"},
	} {
		agentDir := filepath.Join(townRoot, agent.name)
		oldSettings := filepath.Join(agentDir, ".claude", "settings.json")
		if fileExists(oldSettings) {
			files = append(files, staleSettingsInfo{
				path:          oldSettings,
				agentType:     agent.name,
				sessionName:   agent.session,
				wrongLocation: true,
				missing:       []string{"old filename (should be settings.local.json)"},
			})
		}
	}

	// Find rig directories
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return files
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Skip known non-rig directories
		if rigName == "mayor" || rigName == "deacon" || rigName == "daemon" ||
			rigName == ".git" || rigName == "docs" || rigName[0] == '.' {
			continue
		}

		// WITNESS: working dir is witness/ or witness/rig/ (depends on witnessDir())
		// Parent dir settings are stale if witness/rig/ exists (then witness/ is parent)
		witnessDir := filepath.Join(rigPath, "witness")
		witnessRigDir := filepath.Join(witnessDir, "rig")
		witnessRigExists := dirExists(witnessRigDir)

		for _, filename := range settingsFilenames {
			// Settings in witness/ parent dir (stale if witness/rig/ exists)
			witnessParentSettings := filepath.Join(witnessDir, ".claude", filename)
			if fileExists(witnessParentSettings) {
				if witnessRigExists {
					// witness/rig/ exists, so witness/ is parent dir - stale
					files = append(files, staleSettingsInfo{
						path:          witnessParentSettings,
						agentType:     "witness",
						rigName:       rigName,
						sessionName:   fmt.Sprintf("gt-%s-witness", rigName),
						wrongLocation: true,
						missing:       []string{"wrong location (working dir is witness/rig/)"},
					})
				} else if filename == "settings.json" {
					// witness/ is working dir but old filename - stale
					files = append(files, staleSettingsInfo{
						path:          witnessParentSettings,
						agentType:     "witness",
						rigName:       rigName,
						sessionName:   fmt.Sprintf("gt-%s-witness", rigName),
						wrongLocation: true,
						missing:       []string{"old filename (should be settings.local.json)"},
					})
				}
				// settings.local.json in witness/ when witness/rig/ doesn't exist is CORRECT
			}

			// Settings in witness/rig/ working dir
			witnessWorkSettings := filepath.Join(witnessRigDir, ".claude", filename)
			if fileExists(witnessWorkSettings) && filename == "settings.json" {
				// Old filename in working dir - stale
				files = append(files, staleSettingsInfo{
					path:          witnessWorkSettings,
					agentType:     "witness",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-witness", rigName),
					wrongLocation: true,
					missing:       []string{"old filename (should be settings.local.json)"},
				})
			}
		}

		// REFINERY: working dir is refinery/rig/, parent is refinery/
		refineryDir := filepath.Join(rigPath, "refinery")
		refineryRigDir := filepath.Join(refineryDir, "rig")

		for _, filename := range settingsFilenames {
			// Settings in refinery/ parent dir - always stale
			refineryParentSettings := filepath.Join(refineryDir, ".claude", filename)
			if fileExists(refineryParentSettings) {
				files = append(files, staleSettingsInfo{
					path:          refineryParentSettings,
					agentType:     "refinery",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-refinery", rigName),
					wrongLocation: true,
					missing:       []string{"wrong location (working dir is refinery/rig/)"},
				})
			}

			// Settings in refinery/rig/ working dir with old filename - stale
			refineryWorkSettings := filepath.Join(refineryRigDir, ".claude", filename)
			if fileExists(refineryWorkSettings) && filename == "settings.json" {
				files = append(files, staleSettingsInfo{
					path:          refineryWorkSettings,
					agentType:     "refinery",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-refinery", rigName),
					wrongLocation: true,
					missing:       []string{"old filename (should be settings.local.json)"},
				})
			}
		}

		// CREW: working dir is crew/<name>/, parent is crew/
		crewDir := filepath.Join(rigPath, "crew")

		for _, filename := range settingsFilenames {
			// Settings in crew/ parent dir - always stale
			crewParentSettings := filepath.Join(crewDir, ".claude", filename)
			if fileExists(crewParentSettings) {
				files = append(files, staleSettingsInfo{
					path:          crewParentSettings,
					agentType:     "crew",
					rigName:       rigName,
					sessionName:   "",
					wrongLocation: true,
					missing:       []string{"wrong location (working dir is crew/<name>/)"},
				})
			}
		}

		// Check each crew member's working directory for old filename
		if dirExists(crewDir) {
			crewEntries, _ := os.ReadDir(crewDir)
			for _, crewEntry := range crewEntries {
				if !crewEntry.IsDir() || crewEntry.Name() == ".claude" {
					continue
				}
				crewWorkDir := filepath.Join(crewDir, crewEntry.Name())
				oldSettings := filepath.Join(crewWorkDir, ".claude", "settings.json")
				if fileExists(oldSettings) {
					files = append(files, staleSettingsInfo{
						path:          oldSettings,
						agentType:     "crew",
						rigName:       rigName,
						sessionName:   fmt.Sprintf("gt-%s-crew-%s", rigName, crewEntry.Name()),
						wrongLocation: true,
						missing:       []string{"old filename (should be settings.local.json)"},
					})
				}
			}
		}

		// POLECAT: working dir is polecats/<name>/<rig>/, parent is polecats/
		polecatsDir := filepath.Join(rigPath, "polecats")

		for _, filename := range settingsFilenames {
			// Settings in polecats/ parent dir - always stale
			polecatsParentSettings := filepath.Join(polecatsDir, ".claude", filename)
			if fileExists(polecatsParentSettings) {
				files = append(files, staleSettingsInfo{
					path:          polecatsParentSettings,
					agentType:     "polecat",
					rigName:       rigName,
					sessionName:   "",
					wrongLocation: true,
					missing:       []string{"wrong location (working dir is polecats/<name>/<rig>/)"},
				})
			}
		}

		// Check each polecat's directories for stale settings
		if dirExists(polecatsDir) {
			polecatEntries, _ := os.ReadDir(polecatsDir)
			for _, pcEntry := range polecatEntries {
				if !pcEntry.IsDir() || pcEntry.Name() == ".claude" {
					continue
				}
				pcName := pcEntry.Name()

				for _, filename := range settingsFilenames {
					// Settings in polecats/<name>/ (intermediate parent) - stale
					pcParentSettings := filepath.Join(polecatsDir, pcName, ".claude", filename)
					if fileExists(pcParentSettings) {
						files = append(files, staleSettingsInfo{
							path:          pcParentSettings,
							agentType:     "polecat",
							rigName:       rigName,
							sessionName:   fmt.Sprintf("gt-%s-%s", rigName, pcName),
							wrongLocation: true,
							missing:       []string{"wrong location (working dir is polecats/<name>/<rig>/)"},
						})
					}

					// Settings in polecats/<name>/<rig>/ working dir with old filename - stale
					pcWorkSettings := filepath.Join(polecatsDir, pcName, rigName, ".claude", filename)
					if fileExists(pcWorkSettings) && filename == "settings.json" {
						files = append(files, staleSettingsInfo{
							path:          pcWorkSettings,
							agentType:     "polecat",
							rigName:       rigName,
							sessionName:   fmt.Sprintf("gt-%s-%s", rigName, pcName),
							wrongLocation: true,
							missing:       []string{"old filename (should be settings.local.json)"},
						})
					}
				}
			}
		}
	}

	return files
}

// getGitFileStatus determines the git status of a file.
// Returns untracked, tracked-clean, tracked-modified, or unknown.
func (c *ClaudeSettingsCheck) getGitFileStatus(filePath string) gitFileStatus {
	dir := filepath.Dir(filePath)
	fileName := filepath.Base(filePath)

	// Check if we're in a git repo
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return gitStatusUnknown
	}

	// Check if file is tracked
	cmd = exec.Command("git", "-C", dir, "ls-files", fileName)
	output, err := cmd.Output()
	if err != nil {
		return gitStatusUnknown
	}

	if len(strings.TrimSpace(string(output))) == 0 {
		// File is not tracked
		return gitStatusUntracked
	}

	// File is tracked - check if modified
	cmd = exec.Command("git", "-C", dir, "diff", "--quiet", fileName)
	if err := cmd.Run(); err != nil {
		// Non-zero exit means file has changes
		return gitStatusTrackedModified
	}

	// Also check for staged changes
	cmd = exec.Command("git", "-C", dir, "diff", "--cached", "--quiet", fileName)
	if err := cmd.Run(); err != nil {
		return gitStatusTrackedModified
	}

	return gitStatusTrackedClean
}

// Fix deletes stale settings files.
// Agent startup (EnsureSettingsForRole) handles creation of correct settings.
// Files with local modifications are skipped to avoid losing user changes.
func (c *ClaudeSettingsCheck) Fix(ctx *CheckContext) error {
	var errors []string
	var skipped []string
	var deletedTownRoot bool
	var deletedWorkingDir bool

	for _, sf := range c.staleSettings {
		// Skip files with local modifications - require manual review
		if sf.gitStatus == gitStatusTrackedModified {
			skipped = append(skipped, fmt.Sprintf("%s: has local modifications, skipping", sf.path))
			continue
		}

		// Delete the stale file
		if err := os.Remove(sf.path); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete %s: %v", sf.path, err))
			continue
		}

		// Also delete parent .claude directory if empty
		claudeDir := filepath.Dir(sf.path)
		_ = os.Remove(claudeDir) // Best-effort, will fail if not empty

		// Track what kind of file we deleted
		if strings.HasPrefix(sf.path, filepath.Join(ctx.TownRoot, ".claude")) ||
			sf.path == filepath.Join(ctx.TownRoot, "CLAUDE.md") {
			deletedTownRoot = true
		} else if len(sf.missing) > 0 && strings.Contains(sf.missing[0], "old filename") {
			// Deleted old settings.json from a working directory - agent needs restart
			deletedWorkingDir = true
		}
	}

	// Warn about needing to restart agents
	if deletedTownRoot || deletedWorkingDir {
		fmt.Printf("\n  %s Deleted stale settings files. Running agents need restart to pick up new config.\n", style.Warning.Render("⚠"))
		fmt.Printf("      Infrastructure: gt shutdown && gt up\n")
		fmt.Printf("      Crew: gt crew attach <name> (they'll get new settings on next session)\n\n")
	}

	// Report skipped files as warnings, not errors
	if len(skipped) > 0 {
		for _, s := range skipped {
			fmt.Printf("  Warning: %s\n", s)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
