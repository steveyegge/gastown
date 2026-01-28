package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/runtime"
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

// AgentSettingsCheck verifies that agent settings files match the expected templates.
// Supports both .claude and .opencode directories for different providers.
// Detects stale settings files that are missing required hooks or configuration.
type AgentSettingsCheck struct {
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

// NewAgentSettingsCheck creates a new settings validation check.
func NewAgentSettingsCheck() *AgentSettingsCheck {
	return &AgentSettingsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "settings",
				CheckDescription: "Verify settings files match expected templates",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks all settings files for staleness.
func (c *AgentSettingsCheck) Run(ctx *CheckContext) *CheckResult {
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
			Message: "All settings files are up to date",
		}
	}

	fixHint := "Run 'gt doctor --fix' to update settings and restart affected agents"
	if hasModifiedFiles {
		fixHint = "Run 'gt doctor --fix' to fix safe issues. Files with local modifications require manual review."
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d stale config file(s) in wrong location", len(c.staleSettings)),
		Details: details,
		FixHint: fixHint,
	}
}

// knownHooksDirs lists all provider hook directories to check for stale settings.
var knownHooksDirs = []string{".claude", ".opencode"}

// findSettingsFiles locates all settings files and identifies their agent type.
// Checks for both .claude and .opencode directories to support multiple providers.
func (c *AgentSettingsCheck) findSettingsFiles(townRoot string) []staleSettingsInfo {
	var files []staleSettingsInfo

	// Town-level: mayor settings at town root (~/gt/.claude/ or ~/gt/.opencode/)
	// This is CORRECT - Mayor runs from town root, so settings must be here.
	// Neither Claude Code nor OpenCode do directory traversal to find settings.
	for _, hooksDir := range knownHooksDirs {
		mayorTownRootSettings := filepath.Join(townRoot, hooksDir, "settings.json")
		if fileExists(mayorTownRootSettings) {
			files = append(files, staleSettingsInfo{
				path:        mayorTownRootSettings,
				agentType:   "mayor",
				sessionName: "hq-mayor",
				gitStatus:   c.getGitFileStatus(mayorTownRootSettings),
			})
		}
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

	// Note: Mayor settings in ~/gt/mayor/ are legacy (pre-OpenCode).
	// Mayor now runs from town root, so settings there are handled above.

	// Town-level: deacon (~/gt/deacon/<hooksDir>/settings.json)
	deaconDir := filepath.Join(townRoot, "deacon")
	deaconHooksDir := DetectHooksDir(deaconDir)
	deaconSettings := filepath.Join(deaconDir, deaconHooksDir, "settings.json")
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

		// Check for witness settings - witness/<hooksDir>/ is correct (outside git repo)
		// Settings in witness/rig/<hooksDir>/ are wrong (inside source repo)
		witnessDir := filepath.Join(rigPath, "witness")
		witnessHooksDir := DetectHooksDir(witnessDir)
		witnessSettings := filepath.Join(witnessDir, witnessHooksDir, "settings.json")
		if fileExists(witnessSettings) {
			files = append(files, staleSettingsInfo{
				path:        witnessSettings,
				agentType:   "witness",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-witness", rigName),
			})
		}
		// Check for wrong settings inside git repo (both providers)
		for _, hooksDir := range knownHooksDirs {
			witnessWrongSettings := filepath.Join(witnessDir, "rig", hooksDir, "settings.json")
			if fileExists(witnessWrongSettings) {
				files = append(files, staleSettingsInfo{
					path:          witnessWrongSettings,
					agentType:     "witness",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-witness", rigName),
					wrongLocation: true,
				})
			}
		}

		// Check for refinery settings - refinery/<hooksDir>/ is correct (outside git repo)
		// Settings in refinery/rig/<hooksDir>/ are wrong (inside source repo)
		refineryDir := filepath.Join(rigPath, "refinery")
		refineryHooksDir := DetectHooksDir(refineryDir)
		refinerySettings := filepath.Join(refineryDir, refineryHooksDir, "settings.json")
		if fileExists(refinerySettings) {
			files = append(files, staleSettingsInfo{
				path:        refinerySettings,
				agentType:   "refinery",
				rigName:     rigName,
				sessionName: fmt.Sprintf("gt-%s-refinery", rigName),
			})
		}
		// Check for wrong settings inside git repo (both providers)
		for _, hooksDir := range knownHooksDirs {
			refineryWrongSettings := filepath.Join(refineryDir, "rig", hooksDir, "settings.json")
			if fileExists(refineryWrongSettings) {
				files = append(files, staleSettingsInfo{
					path:          refineryWrongSettings,
					agentType:     "refinery",
					rigName:       rigName,
					sessionName:   fmt.Sprintf("gt-%s-refinery", rigName),
					wrongLocation: true,
				})
			}
		}

		// Check for crew settings - crew/<hooksDir>/ is correct (shared by all crew, outside git repos)
		// Settings in crew/<name>/<hooksDir>/ are wrong (inside git repos)
		crewDir := filepath.Join(rigPath, "crew")
		crewHooksDir := DetectHooksDir(crewDir)
		crewSettings := filepath.Join(crewDir, crewHooksDir, "settings.json")
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
				// Skip non-directories and hooks directories
				if !crewEntry.IsDir() {
					continue
				}
				name := crewEntry.Name()
				if name == ".claude" || name == ".opencode" {
					continue
				}
				// Check for wrong settings inside git repos (both providers)
				for _, hooksDir := range knownHooksDirs {
					crewWrongSettings := filepath.Join(crewDir, name, hooksDir, "settings.json")
					if fileExists(crewWrongSettings) {
						files = append(files, staleSettingsInfo{
							path:          crewWrongSettings,
							agentType:     "crew",
							rigName:       rigName,
							sessionName:   fmt.Sprintf("gt-%s-crew-%s", rigName, name),
							wrongLocation: true,
						})
					}
				}
			}
		}

		// Check for polecat settings - polecats/<hooksDir>/ is correct (shared by all polecats, outside git repos)
		// Settings in polecats/<name>/<hooksDir>/ are wrong (inside git repos)
		polecatsDir := filepath.Join(rigPath, "polecats")
		polecatsHooksDir := DetectHooksDir(polecatsDir)
		polecatsSettings := filepath.Join(polecatsDir, polecatsHooksDir, "settings.json")
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
				// Skip non-directories and hooks directories
				if !pcEntry.IsDir() {
					continue
				}
				pcName := pcEntry.Name()
				if pcName == ".claude" || pcName == ".opencode" {
					continue
				}
				// Check for wrong settings in both structures (both providers):
				// Old structure: polecats/<name>/<hooksDir>/settings.json
				// New structure: polecats/<name>/<rigname>/<hooksDir>/settings.json
				for _, hooksDir := range knownHooksDirs {
					wrongPaths := []string{
						filepath.Join(polecatsDir, pcName, hooksDir, "settings.json"),
						filepath.Join(polecatsDir, pcName, rigName, hooksDir, "settings.json"),
					}
					for _, pcWrongSettings := range wrongPaths {
						if fileExists(pcWrongSettings) {
							files = append(files, staleSettingsInfo{
								path:          pcWrongSettings,
								agentType:     "polecat",
								rigName:       rigName,
								sessionName:   fmt.Sprintf("gt-%s-%s", rigName, pcName),
								wrongLocation: true,
							})
						}
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
func (c *AgentSettingsCheck) checkSettings(path, _ string) []string {
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
func (c *AgentSettingsCheck) getGitFileStatus(filePath string) gitFileStatus {
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
func (c *AgentSettingsCheck) hookHasPattern(hooks map[string]any, hookName, pattern string) bool {
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
func (c *AgentSettingsCheck) Fix(ctx *CheckContext) error {
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

		// Also delete parent hooks directory if empty
		hooksDir := filepath.Dir(sf.path)
		_ = os.Remove(hooksDir) // Best-effort, will fail if not empty

		// For files in wrong locations, delete and create at correct location
		if sf.wrongLocation {
			mayorDir := filepath.Join(ctx.TownRoot, "mayor")

			// For mayor CLAUDE.md at town root, create at mayor/
			// Note: Mayor settings.json at town root are now CORRECT (Mayor runs from townRoot)
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

			fmt.Printf("\n  %s File was in wrong location and has been moved.\n", style.Warning.Render("⚠"))
			fmt.Printf("      Restart affected agents to pick up new config: gt up --restart\n\n")
			continue
		}

		// Recreate settings using runtime.EnsureSettingsForRole for OpenCode support
		workDir := filepath.Dir(hooksDir) // agent work directory
		runtimeConfig := config.ResolveAgentConfig(ctx.TownRoot, workDir)
		if err := runtime.EnsureSettingsForRole(workDir, sf.agentType, runtimeConfig); err != nil {
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
