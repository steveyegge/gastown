package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/claude"
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
				CheckDescription: "Verify Claude settings.json files match expected templates",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks all Claude settings.json files for staleness.
func (c *ClaudeSettingsCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleSettings = nil

	var details []string
	var hasModifiedFiles bool

	// Find all settings.json files
	settingsFiles := c.findSettingsFiles(ctx.TownRoot)

	for _, sf := range settingsFiles {
		// Files in wrong locations are always stale (should be deleted)
		if sf.wrongLocation {
			// Check git status to determine safe deletion strategy
			sf.gitStatus = c.getGitFileStatus(sf.path)
			c.staleSettings = append(c.staleSettings, sf)

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
			details = append(details, fmt.Sprintf("%s: missing %s", sf.path, strings.Join(missing, ", ")))
		}
	}

	if len(c.staleSettings) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All Claude settings.json files are up to date",
		}
	}

	fixHint := "Run 'gt doctor --fix' to update settings and restart affected agents"
	if hasModifiedFiles {
		fixHint = "Run 'gt doctor --fix' to fix safe issues. Files with local modifications require manual review."
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d stale Claude config file(s) in wrong location", len(c.staleSettings)),
		Details: details,
		FixHint: fixHint,
	}
}

// findSettingsFiles locates all .claude/settings.json files and identifies their agent type.
func (c *ClaudeSettingsCheck) findSettingsFiles(townRoot string) []staleSettingsInfo {
	var files []staleSettingsInfo

	// Check for STALE settings at town root (~/gt/.claude/settings.json)
	// This is WRONG - settings here pollute ALL child workspaces via directory traversal.
	// Mayor settings should be at ~/gt/mayor/.claude/ instead.
	staleTownRootSettings := filepath.Join(townRoot, ".claude", "settings.json")
	if fileExists(staleTownRootSettings) {
		files = append(files, staleSettingsInfo{
			path:          staleTownRootSettings,
			agentType:     "mayor",
			sessionName:   "hq-mayor",
			wrongLocation: true,
			gitStatus:     c.getGitFileStatus(staleTownRootSettings),
			missing:       []string{"should be at mayor/.claude/settings.json, not town root"},
		})
	}

	// Check for STALE CLAUDE.md at town root (~/gt/CLAUDE.md)
	// This is WRONG - CLAUDE.md here is inherited by ALL agents via directory traversal,
	// causing crew/polecat/etc to receive Mayor-specific instructions.
	// Mayor's CLAUDE.md should be at ~/gt/mayor/CLAUDE.md instead.
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

	// Town-level: mayor (~/gt/mayor/.claude/settings.json) - CORRECT location
	mayorSettings := filepath.Join(townRoot, "mayor", ".claude", "settings.json")
	if fileExists(mayorSettings) {
		files = append(files, staleSettingsInfo{
			path:        mayorSettings,
			agentType:   "mayor",
			sessionName: "hq-mayor",
		})
	}

	// Town-level: deacon (~/gt/deacon/.claude/settings.json)
	deaconSettings := filepath.Join(townRoot, "deacon", ".claude", "settings.json")
	if fileExists(deaconSettings) {
		files = append(files, staleSettingsInfo{
			path:        deaconSettings,
			agentType:   "deacon",
			sessionName: "hq-deacon",
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

		// Check for witness settings - witness/.claude/ is correct (outside git repo)
		// Settings in witness/rig/.claude/ are wrong (inside source repo)
		witnessSettings := filepath.Join(rigPath, "witness", ".claude", "settings.json")
		if fileExists(witnessSettings) {
			files = append(files, staleSettingsInfo{
				path:        witnessSettings,
				agentType:   "witness",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-witness", rigName),
			})
		}
		witnessWrongSettings := filepath.Join(rigPath, "witness", "rig", ".claude", "settings.json")
		if fileExists(witnessWrongSettings) {
			files = append(files, staleSettingsInfo{
				path:          witnessWrongSettings,
				agentType:     "witness",
				rigName:       rigName,
				sessionName:   fmt.Sprintf("gt-%s-witness", rigName),
				wrongLocation: true,
			})
		}

		// Check for refinery settings - refinery/.claude/ is correct (outside git repo)
		// Settings in refinery/rig/.claude/ are wrong (inside source repo)
		refinerySettings := filepath.Join(rigPath, "refinery", ".claude", "settings.json")
		if fileExists(refinerySettings) {
			files = append(files, staleSettingsInfo{
				path:        refinerySettings,
				agentType:   "refinery",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-refinery", rigName),
			})
		}
		refineryWrongSettings := filepath.Join(rigPath, "refinery", "rig", ".claude", "settings.json")
		if fileExists(refineryWrongSettings) {
			files = append(files, staleSettingsInfo{
				path:          refineryWrongSettings,
				agentType:     "refinery",
				rigName:       rigName,
				sessionName:   fmt.Sprintf("gt-%s-refinery", rigName),
				wrongLocation: true,
			})
		}

		// Check for crew settings - crew/.claude/ is correct (shared by all crew, outside git repos)
		// Settings in crew/<name>/.claude/ are wrong (inside git repos)
		crewDir := filepath.Join(rigPath, "crew")
		crewSettings := filepath.Join(crewDir, ".claude", "settings.json")
		if fileExists(crewSettings) {
			files = append(files, staleSettingsInfo{
				path:        crewSettings,
				agentType:   "crew",
				rigName:     rigName,
				sessionName: "", // Shared settings, no single session
			})
		}
		if dirExists(crewDir) {
			crewEntries, _ := os.ReadDir(crewDir)
			for _, crewEntry := range crewEntries {
				if !crewEntry.IsDir() || crewEntry.Name() == ".claude" {
					continue
				}
				crewWrongSettings := filepath.Join(crewDir, crewEntry.Name(), ".claude", "settings.json")
				if fileExists(crewWrongSettings) {
					files = append(files, staleSettingsInfo{
						path:          crewWrongSettings,
						agentType:     "crew",
						rigName:       rigName,
						sessionName:   fmt.Sprintf("gt-%s-crew-%s", rigName, crewEntry.Name()),
						wrongLocation: true,
					})
				}
			}
		}

		// Check for polecat settings - polecats/.claude/ is correct (shared by all polecats, outside git repos)
		// Settings in polecats/<name>/.claude/ are wrong (inside git repos)
		polecatsDir := filepath.Join(rigPath, "polecats")
		polecatsSettings := filepath.Join(polecatsDir, ".claude", "settings.json")
		if fileExists(polecatsSettings) {
			files = append(files, staleSettingsInfo{
				path:        polecatsSettings,
				agentType:   "polecat",
				rigName:     rigName,
				sessionName: "", // Shared settings, no single session
			})
		}
		if dirExists(polecatsDir) {
			polecatEntries, _ := os.ReadDir(polecatsDir)
			for _, pcEntry := range polecatEntries {
				if !pcEntry.IsDir() || pcEntry.Name() == ".claude" {
					continue
				}
				// Check for wrong settings in both structures:
				// Old structure: polecats/<name>/.claude/settings.json
				// New structure: polecats/<name>/<rigname>/.claude/settings.json
				wrongPaths := []string{
					filepath.Join(polecatsDir, pcEntry.Name(), ".claude", "settings.json"),
					filepath.Join(polecatsDir, pcEntry.Name(), rigName, ".claude", "settings.json"),
				}
				for _, pcWrongSettings := range wrongPaths {
					if fileExists(pcWrongSettings) {
						files = append(files, staleSettingsInfo{
							path:          pcWrongSettings,
							agentType:     "polecat",
							rigName:       rigName,
							sessionName:   fmt.Sprintf("gt-%s-%s", rigName, pcEntry.Name()),
							wrongLocation: true,
						})
					}
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

// Fix deletes stale settings files and restarts affected agents.
// Files with local modifications are skipped to avoid losing user changes.
func (c *ClaudeSettingsCheck) Fix(ctx *CheckContext) error {
	var errors []string
	var skipped []string
	t := tmux.NewTmux()

	for _, sf := range c.staleSettings {
		// Skip files with local modifications - require manual review
		if sf.wrongLocation && sf.gitStatus == gitStatusTrackedModified {
			skipped = append(skipped, fmt.Sprintf("%s: has local modifications, skipping", sf.path))
			continue
		}

		// Delete the stale settings file
		if err := os.Remove(sf.path); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete %s: %v", sf.path, err))
			continue
		}

		// Also delete parent .claude directory if empty
		claudeDir := filepath.Dir(sf.path)
		_ = os.Remove(claudeDir) // Best-effort, will fail if not empty

		// For files in wrong locations, delete and create at correct location
		if sf.wrongLocation {
			mayorDir := filepath.Join(ctx.TownRoot, "mayor")

			// For mayor settings.json at town root, create at mayor/.claude/
			if sf.agentType == "mayor" && strings.HasSuffix(claudeDir, ".claude") && !strings.Contains(sf.path, "/mayor/") {
				if err := os.MkdirAll(mayorDir, 0755); err == nil {
					_ = claude.EnsureSettingsForRole(mayorDir, "mayor")
				}
			}

			// For mayor CLAUDE.md at town root, create at mayor/
			if sf.agentType == "mayor" && strings.HasSuffix(sf.path, "CLAUDE.md") && !strings.Contains(sf.path, "/mayor/") {
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

			// Town-root files were inherited by ALL agents via directory traversal.
			// Warn user to restart agents - don't auto-kill sessions as that's too disruptive,
			// especially since deacon runs gt doctor automatically which would create a loop.
			// Settings are only read at startup, so running agents already have config loaded.
			fmt.Printf("\n  %s Town-root settings were moved. Restart agents to pick up new config:\n", style.Warning.Render("⚠"))
			fmt.Printf("      gt up --restart\n\n")
			continue
		}

		// Recreate settings using EnsureSettingsForRole
		workDir := filepath.Dir(claudeDir) // agent work directory
		if err := claude.EnsureSettingsForRole(workDir, sf.agentType); err != nil {
			errors = append(errors, fmt.Sprintf("failed to recreate settings for %s: %v", sf.path, err))
			continue
		}

		// Only cycle patrol roles if --restart-sessions was explicitly passed.
		// This prevents unexpected session restarts during routine --fix operations.
		// Crew and polecats are spawned on-demand and won't auto-restart anyway.
		if ctx.RestartSessions {
			if sf.agentType == "witness" || sf.agentType == "refinery" ||
				sf.agentType == "deacon" || sf.agentType == "mayor" {
				running, _ := t.HasSession(sf.sessionName)
				if running {
					// Cycle the agent by killing and letting gt up restart it
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
