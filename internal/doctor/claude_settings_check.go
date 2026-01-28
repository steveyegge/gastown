package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// gitFileStatus represents the git status of a file.
type gitFileStatus string

const (
	gitStatusUntracked       gitFileStatus = "untracked"        // File not tracked by git
	gitStatusTrackedClean    gitFileStatus = "tracked-clean"    // Tracked, no local modifications
	gitStatusTrackedModified gitFileStatus = "tracked-modified" // Tracked with local modifications
	gitStatusUnknown         gitFileStatus = "unknown"          // Not in a git repo or error
)

// ClaudeSettingsCheck verifies that Claude settings.json files match the expected templates.
// Detects stale settings files that are missing required hooks or configuration.
type ClaudeSettingsCheck struct {
	FixableCheck
	staleSettings []staleSettingsInfo
}

type staleSettingsInfo struct {
	path           string        // Full path to settings file
	agentType      string        // e.g., "witness", "refinery", "deacon", "mayor"
	rigName        string        // Rig name (empty for town-level agents)
	sessionName    string        // tmux session name for cycling
	missing        []string      // What's missing from the settings
	wrongLocation  bool          // True if file is in wrong location (should be deleted)
	missingFile    bool          // True if settings.local.json doesn't exist (needs agent restart)
	gitStatus      gitFileStatus // Git status for wrong-location files (for safe deletion)
}

// NewClaudeSettingsCheck creates a new Claude settings validation check.
func NewClaudeSettingsCheck() *ClaudeSettingsCheck {
	return &ClaudeSettingsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "claude-settings",
				CheckDescription: "Verify Claude settings.json files match expected templates",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks all Claude settings files for staleness or missing settings.local.json.
func (c *ClaudeSettingsCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleSettings = nil

	var details []string
	var hasModifiedFiles bool
	var hasMissingFiles bool
	var hasStaleFiles bool

	// Find all settings files (stale and missing)
	settingsFiles := c.findSettingsFiles(ctx.TownRoot)

	for _, sf := range settingsFiles {
		// Missing settings.local.json files need agent restart to create
		if sf.missingFile {
			c.staleSettings = append(c.staleSettings, sf)
			details = append(details, fmt.Sprintf("%s: missing (restart %s to create)", sf.path, sf.agentType))
			hasMissingFiles = true
			continue
		}

		// Files in wrong locations are always stale (should be deleted)
		if sf.wrongLocation {
			// Check git status to determine safe deletion strategy
			sf.gitStatus = c.getGitFileStatus(sf.path)
			c.staleSettings = append(c.staleSettings, sf)
			hasStaleFiles = true

			// Provide detailed message based on git status
			var statusMsg string
			switch sf.gitStatus {
			case gitStatusUntracked:
				statusMsg = "wrong location, untracked (safe to delete)"
			case gitStatusTrackedClean:
				statusMsg = "wrong location, tracked but unmodified (safe to delete)"
			case gitStatusTrackedModified:
				statusMsg = "wrong location, tracked with local modifications (manual review needed)"
				hasModifiedFiles = true
			default:
				statusMsg = "wrong location (inside source repo)"
			}
			details = append(details, fmt.Sprintf("%s: %s", sf.path, statusMsg))
			continue
		}

		// Check content of files in correct locations
		missing := c.checkSettings(sf.path, sf.agentType)
		if len(missing) > 0 {
			sf.missing = missing
			c.staleSettings = append(c.staleSettings, sf)
			hasStaleFiles = true
			details = append(details, fmt.Sprintf("%s: missing %s", sf.path, strings.Join(missing, ", ")))
		}
	}

	if len(c.staleSettings) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All Claude settings.local.json files are up to date",
		}
	}

	// Build appropriate message and fix hint
	var message string
	var fixHint string

	if hasMissingFiles && !hasStaleFiles {
		message = fmt.Sprintf("Found %d agent(s) missing settings.local.json", len(c.staleSettings))
		fixHint = "Run 'gt up --restart' to restart agents and create settings"
	} else if hasStaleFiles && !hasMissingFiles {
		message = fmt.Sprintf("Found %d stale Claude config file(s)", len(c.staleSettings))
		if hasModifiedFiles {
			fixHint = "Run 'gt doctor --fix' to fix safe issues. Files with local modifications require manual review."
		} else {
			fixHint = "Run 'gt doctor --fix' to delete stale files, then 'gt up --restart' to create new settings"
		}
	} else {
		message = fmt.Sprintf("Found %d Claude settings issue(s)", len(c.staleSettings))
		if hasModifiedFiles {
			fixHint = "Run 'gt doctor --fix' to fix safe issues, then 'gt up --restart'. Files with local modifications require manual review."
		} else {
			fixHint = "Run 'gt doctor --fix' to delete stale files, then 'gt up --restart' to create new settings"
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: message,
		Details: details,
		FixHint: fixHint,
	}
}

// findSettingsFiles locates all .claude/settings.json files and identifies their agent type.
// All settings.json files are now considered stale - we use settings.local.json instead.
// See: https://github.com/anthropics/claude-code/issues/12962
func (c *ClaudeSettingsCheck) findSettingsFiles(townRoot string) []staleSettingsInfo {
	var files []staleSettingsInfo

	// Check for STALE settings at town root (~/gt/.claude/settings.json)
	// This is WRONG - settings here pollute ALL child workspaces via directory traversal.
	staleTownRootSettings := filepath.Join(townRoot, ".claude", "settings.json")
	if fileExists(staleTownRootSettings) {
		files = append(files, staleSettingsInfo{
			path:          staleTownRootSettings,
			agentType:     "mayor",
			sessionName:   "hq-mayor",
			wrongLocation: true,
			gitStatus:     c.getGitFileStatus(staleTownRootSettings),
			missing:       []string{"stale settings.json at town root (should not exist)"},
		})
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
			gitStatus:     c.getGitFileStatus(staleTownRootCLAUDEmd),
			missing:       []string{"should be at mayor/CLAUDE.md, not town root"},
		})
	}

	// Town-level: mayor - check for stale settings.json (should be settings.local.json)
	mayorStaleSettings := filepath.Join(townRoot, "mayor", ".claude", "settings.json")
	if fileExists(mayorStaleSettings) {
		files = append(files, staleSettingsInfo{
			path:          mayorStaleSettings,
			agentType:     "mayor",
			sessionName:   "hq-mayor",
			wrongLocation: true,
			missing:       []string{"stale settings.json (should be settings.local.json)"},
		})
	}
	// Check for correct settings.local.json
	mayorSettings := filepath.Join(townRoot, "mayor", ".claude", "settings.local.json")
	mayorWorkDir := filepath.Join(townRoot, "mayor")
	if fileExists(mayorSettings) {
		files = append(files, staleSettingsInfo{
			path:        mayorSettings,
			agentType:   "mayor",
			sessionName: "hq-mayor",
		})
	} else if dirExists(mayorWorkDir) {
		// Working directory exists but settings.local.json is missing
		files = append(files, staleSettingsInfo{
			path:        mayorSettings,
			agentType:   "mayor",
			sessionName: "hq-mayor",
			missingFile: true,
		})
	}

	// Town-level: deacon - check for stale settings.json (should be settings.local.json)
	deaconStaleSettings := filepath.Join(townRoot, "deacon", ".claude", "settings.json")
	if fileExists(deaconStaleSettings) {
		files = append(files, staleSettingsInfo{
			path:          deaconStaleSettings,
			agentType:     "deacon",
			sessionName:   "hq-deacon",
			wrongLocation: true,
			missing:       []string{"stale settings.json (should be settings.local.json)"},
		})
	}
	// Check for correct settings.local.json
	deaconSettings := filepath.Join(townRoot, "deacon", ".claude", "settings.local.json")
	deaconWorkDir := filepath.Join(townRoot, "deacon")
	if fileExists(deaconSettings) {
		files = append(files, staleSettingsInfo{
			path:        deaconSettings,
			agentType:   "deacon",
			sessionName: "hq-deacon",
		})
	} else if dirExists(deaconWorkDir) {
		// Working directory exists but settings.local.json is missing
		files = append(files, staleSettingsInfo{
			path:        deaconSettings,
			agentType:   "deacon",
			sessionName: "hq-deacon",
			missingFile: true,
		})
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

		// Check for witness settings
		// STALE: witness/.claude/settings.json (parent directory - Claude doesn't traverse)
		// STALE: witness/.claude/settings.local.json (parent directory)
		// STALE: witness/rig/.claude/settings.json (old filename)
		// CORRECT: witness/rig/.claude/settings.local.json
		for _, staleWitnessPath := range []string{
			filepath.Join(rigPath, "witness", ".claude", "settings.json"),
			filepath.Join(rigPath, "witness", ".claude", "settings.local.json"),
			filepath.Join(rigPath, "witness", "rig", ".claude", "settings.json"),
		} {
			if fileExists(staleWitnessPath) {
				files = append(files, staleSettingsInfo{
					path:          staleWitnessPath,
					agentType:     "witness",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-witness", rigName),
					wrongLocation: true,
					missing:       []string{"stale settings (should be witness/rig/.claude/settings.local.json)"},
				})
			}
		}
		witnessCorrectSettings := filepath.Join(rigPath, "witness", "rig", ".claude", "settings.local.json")
		witnessWorkDir := filepath.Join(rigPath, "witness", "rig")
		if fileExists(witnessCorrectSettings) {
			files = append(files, staleSettingsInfo{
				path:        witnessCorrectSettings,
				agentType:   "witness",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-witness", rigName),
			})
		} else if dirExists(witnessWorkDir) {
			// Working directory exists but settings.local.json is missing
			// Report this even if there's a stale settings.json - user needs to know both issues
			files = append(files, staleSettingsInfo{
				path:        witnessCorrectSettings,
				agentType:   "witness",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-witness", rigName),
				missingFile: true,
			})
		}

		// Check for refinery settings (same pattern as witness)
		for _, staleRefineryPath := range []string{
			filepath.Join(rigPath, "refinery", ".claude", "settings.json"),
			filepath.Join(rigPath, "refinery", ".claude", "settings.local.json"),
			filepath.Join(rigPath, "refinery", "rig", ".claude", "settings.json"),
		} {
			if fileExists(staleRefineryPath) {
				files = append(files, staleSettingsInfo{
					path:          staleRefineryPath,
					agentType:     "refinery",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-refinery", rigName),
					wrongLocation: true,
					missing:       []string{"stale settings (should be refinery/rig/.claude/settings.local.json)"},
				})
			}
		}
		refineryCorrectSettings := filepath.Join(rigPath, "refinery", "rig", ".claude", "settings.local.json")
		refineryWorkDir := filepath.Join(rigPath, "refinery", "rig")
		if fileExists(refineryCorrectSettings) {
			files = append(files, staleSettingsInfo{
				path:        refineryCorrectSettings,
				agentType:   "refinery",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-refinery", rigName),
			})
		} else if dirExists(refineryWorkDir) {
			// Working directory exists but settings.local.json is missing
			files = append(files, staleSettingsInfo{
				path:        refineryCorrectSettings,
				agentType:   "refinery",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-refinery", rigName),
				missingFile: true,
			})
		}

		// Check for crew settings
		// STALE: crew/.claude/settings.json (parent directory)
		// STALE: crew/.claude/settings.local.json (parent directory)
		crewDir := filepath.Join(rigPath, "crew")
		for _, staleCrewPath := range []string{
			filepath.Join(crewDir, ".claude", "settings.json"),
			filepath.Join(crewDir, ".claude", "settings.local.json"),
		} {
			if fileExists(staleCrewPath) {
				files = append(files, staleSettingsInfo{
					path:          staleCrewPath,
					agentType:     "crew",
					rigName:       rigName,
					wrongLocation: true,
					missing:       []string{"stale settings in parent directory (should be in crew/<name>/)"},
				})
			}
		}
		// Check individual crew workers for stale settings.json (should be settings.local.json)
		if dirExists(crewDir) {
			crewEntries, _ := os.ReadDir(crewDir)
			for _, crewEntry := range crewEntries {
				if !crewEntry.IsDir() || crewEntry.Name() == ".claude" {
					continue
				}
				crewStaleSettings := filepath.Join(crewDir, crewEntry.Name(), ".claude", "settings.json")
				if fileExists(crewStaleSettings) {
					files = append(files, staleSettingsInfo{
						path:          crewStaleSettings,
						agentType:     "crew",
						rigName:       rigName,
						sessionName:   fmt.Sprintf("gt-%s-crew-%s", rigName, crewEntry.Name()),
						wrongLocation: true,
						missing:       []string{"stale settings.json (should be settings.local.json)"},
					})
				}
				// Check for correct settings.local.json in crew working directory
				crewCorrectSettings := filepath.Join(crewDir, crewEntry.Name(), ".claude", "settings.local.json")
				crewWorkDir := filepath.Join(crewDir, crewEntry.Name())
				if fileExists(crewCorrectSettings) {
					files = append(files, staleSettingsInfo{
						path:        crewCorrectSettings,
						agentType:   "crew",
						rigName:     rigName,
						sessionName: fmt.Sprintf("gt-%s-crew-%s", rigName, crewEntry.Name()),
					})
				} else if dirExists(crewWorkDir) {
					// Working directory exists but settings.local.json is missing
					files = append(files, staleSettingsInfo{
						path:        crewCorrectSettings,
						agentType:   "crew",
						rigName:     rigName,
						sessionName: fmt.Sprintf("gt-%s-crew-%s", rigName, crewEntry.Name()),
						missingFile: true,
					})
				}
			}
		}

		// Check for polecat settings
		// STALE: polecats/.claude/settings.json (parent directory)
		// STALE: polecats/.claude/settings.local.json (parent directory)
		polecatsDir := filepath.Join(rigPath, "polecats")
		for _, stalePolecatPath := range []string{
			filepath.Join(polecatsDir, ".claude", "settings.json"),
			filepath.Join(polecatsDir, ".claude", "settings.local.json"),
		} {
			if fileExists(stalePolecatPath) {
				files = append(files, staleSettingsInfo{
					path:          stalePolecatPath,
					agentType:     "polecat",
					rigName:       rigName,
					wrongLocation: true,
					missing:       []string{"stale settings in parent directory (should be in polecats/<name>/<rig>/)"},
				})
			}
		}
		// Check individual polecats for stale settings
		if dirExists(polecatsDir) {
			polecatEntries, _ := os.ReadDir(polecatsDir)
			for _, pcEntry := range polecatEntries {
				if !pcEntry.IsDir() || pcEntry.Name() == ".claude" {
					continue
				}
				// Check for stale settings in various locations
				stalePaths := []string{
					filepath.Join(polecatsDir, pcEntry.Name(), ".claude", "settings.json"),
					filepath.Join(polecatsDir, pcEntry.Name(), ".claude", "settings.local.json"),
					filepath.Join(polecatsDir, pcEntry.Name(), rigName, ".claude", "settings.json"),
				}
				for _, stalePath := range stalePaths {
					if fileExists(stalePath) {
						files = append(files, staleSettingsInfo{
							path:          stalePath,
							agentType:     "polecat",
							rigName:       rigName,
							sessionName:   fmt.Sprintf("gt-%s-%s", rigName, pcEntry.Name()),
							wrongLocation: true,
							missing:       []string{"stale settings (should be settings.local.json in worktree)"},
						})
					}
				}
				// Check for correct settings.local.json in polecat worktree
				pcCorrectSettings := filepath.Join(polecatsDir, pcEntry.Name(), rigName, ".claude", "settings.local.json")
				pcWorkDir := filepath.Join(polecatsDir, pcEntry.Name(), rigName)
				if fileExists(pcCorrectSettings) {
					files = append(files, staleSettingsInfo{
						path:        pcCorrectSettings,
						agentType:   "polecat",
						rigName:     rigName,
						sessionName: fmt.Sprintf("gt-%s-%s", rigName, pcEntry.Name()),
					})
				} else if dirExists(pcWorkDir) {
					// Worktree exists but settings.local.json is missing
					files = append(files, staleSettingsInfo{
						path:        pcCorrectSettings,
						agentType:   "polecat",
						rigName:     rigName,
						sessionName: fmt.Sprintf("gt-%s-%s", rigName, pcEntry.Name()),
						missingFile: true,
					})
				}
			}
		}
	}

	return files
}

// checkSettings compares a settings file against the expected template.
// Returns a list of what's missing.
// agentType is reserved for future role-specific validation.
func (c *ClaudeSettingsCheck) checkSettings(path, _ string) []string {
	var missing []string

	// Read the actual settings
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{"unreadable"}
	}

	var actual map[string]any
	if err := json.Unmarshal(data, &actual); err != nil {
		return []string{"invalid JSON"}
	}

	// Check for required elements based on template
	// All templates should have:
	// 1. enabledPlugins
	// 2. PATH export in hooks
	// 3. Stop hook with gt costs record (for autonomous)
	// 4. gt nudge deacon session-started in SessionStart

	// Check enabledPlugins
	if _, ok := actual["enabledPlugins"]; !ok {
		missing = append(missing, "enabledPlugins")
	}

	// Check hooks
	hooks, ok := actual["hooks"].(map[string]any)
	if !ok {
		return append(missing, "hooks")
	}

	// Check SessionStart hook has PATH export
	if !c.hookHasPattern(hooks, "SessionStart", "PATH=") {
		missing = append(missing, "PATH export")
	}

	// Check SessionStart hook has deacon nudge
	if !c.hookHasPattern(hooks, "SessionStart", "gt nudge deacon session-started") {
		missing = append(missing, "deacon nudge")
	}

	// Check SessionStart hook uses --hook flag for session_id passthrough
	// This aligns with the session-hooks check in config_check.go
	if !c.hookHasPattern(hooks, "SessionStart", "--hook") {
		missing = append(missing, "--hook flag")
	}

	// Check Stop hook exists with gt costs record (for all roles)
	if !c.hookHasPattern(hooks, "Stop", "gt costs record") {
		missing = append(missing, "Stop hook")
	}

	return missing
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

// hookHasPattern checks if a hook contains a specific pattern.
func (c *ClaudeSettingsCheck) hookHasPattern(hooks map[string]any, hookName, pattern string) bool {
	hookList, ok := hooks[hookName].([]any)
	if !ok {
		return false
	}

	for _, hook := range hookList {
		hookMap, ok := hook.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, inner := range innerHooks {
			innerMap, ok := inner.(map[string]any)
			if !ok {
				continue
			}
			cmd, ok := innerMap["command"].(string)
			if ok && strings.Contains(cmd, pattern) {
				return true
			}
		}
	}
	return false
}

// Fix deletes stale settings files. Agents auto-install correct settings on restart.
// Files with local modifications are skipped to avoid losing user changes.
func (c *ClaudeSettingsCheck) Fix(ctx *CheckContext) error {
	var errors []string
	var skipped []string
	var needsRestart bool
	t := tmux.NewTmux()

	for _, sf := range c.staleSettings {
		// Skip files that aren't stale (correct settings.local.json files)
		if !sf.wrongLocation && len(sf.missing) == 0 {
			continue
		}

		// Skip files with local modifications - require manual review
		if sf.gitStatus == gitStatusTrackedModified {
			skipped = append(skipped, fmt.Sprintf("%s: has local modifications, skipping", sf.path))
			continue
		}

		// Delete the stale settings file
		if err := os.Remove(sf.path); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete %s: %v", sf.path, err))
			continue
		}
		fmt.Printf("  Deleted stale: %s\n", sf.path)
		needsRestart = true

		// Also delete parent .claude directory if empty
		claudeDir := filepath.Dir(sf.path)
		_ = os.Remove(claudeDir) // Best-effort, will fail if not empty

		// For mayor CLAUDE.md at town root, move it to mayor/
		if sf.agentType == "mayor" && strings.HasSuffix(sf.path, "CLAUDE.md") && !strings.Contains(sf.path, "/mayor/") {
			mayorDir := filepath.Join(ctx.TownRoot, "mayor")
			townName, _ := workspace.GetTownName(ctx.TownRoot)
			if err := templates.CreateMayorCLAUDEmd(
				mayorDir,
				ctx.TownRoot,
				townName,
				session.MayorSessionName(),
				session.DeaconSessionName(),
			); err != nil {
				errors = append(errors, fmt.Sprintf("failed to create mayor/CLAUDE.md: %v", err))
			}
		}

		// Only cycle patrol roles if --restart-sessions was explicitly passed.
		// This prevents unexpected session restarts during routine --fix operations.
		// Crew and polecats are spawned on-demand and won't auto-restart anyway.
		if ctx.RestartSessions {
			if sf.agentType == "witness" || sf.agentType == "refinery" ||
				sf.agentType == "deacon" || sf.agentType == "mayor" {
				running, _ := t.HasSession(sf.sessionName)
				if running {
					// Cycle the agent by killing and letting gt up restart it.
					// Use KillSessionWithProcesses to ensure all descendant processes are killed.
					_ = t.KillSessionWithProcesses(sf.sessionName)
				}
			}
		}
	}

	// Report skipped files as warnings, not errors
	if len(skipped) > 0 {
		for _, s := range skipped {
			fmt.Printf("  Warning: %s\n", s)
		}
	}

	// Tell user to restart agents so they create correct settings
	if needsRestart && !ctx.RestartSessions {
		fmt.Printf("\n  %s Restart agents to create new settings:\n", style.Warning.Render("⚠"))
		fmt.Printf("      gt up --restart\n\n")
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
