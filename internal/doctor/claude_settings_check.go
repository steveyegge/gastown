package doctor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	templateDrift bool          // True if patterns are valid but template hash differs
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
		} else {
			// Patterns are valid - check if template has drifted
			if !c.checkSettingsFreshness(sf.path, sf.agentType) {
				sf.templateDrift = true
				c.staleSettings = append(c.staleSettings, sf)
				details = append(details, fmt.Sprintf("%s: template drift (patterns valid but settings outdated)", sf.path))
			}
		}
	}

	if len(c.staleSettings) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All Claude settings.json files are up to date",
		}
	}

	// Categorize issues: template drift only = warning, wrong location/missing = error
	var hasErrors bool
	var driftCount int
	for _, sf := range c.staleSettings {
		if sf.wrongLocation || len(sf.missing) > 0 {
			hasErrors = true
		}
		if sf.templateDrift {
			driftCount++
		}
	}

	status := StatusWarning
	message := fmt.Sprintf("Found %d settings file(s) with template drift", driftCount)
	fixHint := "Run 'gt doctor --fix' to update settings from templates"

	if hasErrors {
		status = StatusError
		message = fmt.Sprintf("Found %d stale Claude config file(s)", len(c.staleSettings))
		fixHint = "Run 'gt doctor --fix' to update settings and restart affected agents"
	}

	if hasModifiedFiles {
		fixHint = "Run 'gt doctor --fix' to fix issues. Files with local modifications will be renamed to .bak files."
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  status,
		Message: message,
		Details: details,
		FixHint: fixHint,
	}
}

// findSettingsFiles locates all .claude/settings.json files and identifies their agent type.
func (c *ClaudeSettingsCheck) findSettingsFiles(townRoot string) []staleSettingsInfo {
	var files []staleSettingsInfo

	// Check for STALE settings at town root (~/gt/.claude/settings.json)
	// A regular file here is WRONG - it pollutes ALL child workspaces via directory traversal.
	// Mayor settings should be at ~/gt/mayor/.claude/ instead.
	// However, a symlink pointing to mayor/.claude/settings.json is CORRECT -
	// the mayor session runs from town root and needs this symlink to find its hooks.
	staleTownRootSettings := filepath.Join(townRoot, ".claude", "settings.json")
	if fileExists(staleTownRootSettings) {
		isValidSymlink := false
		if info, err := os.Lstat(staleTownRootSettings); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(staleTownRootSettings); err == nil {
				expectedTarget := filepath.Join("..", "mayor", ".claude", "settings.json")
				if target == expectedTarget {
					isValidSymlink = true
				}
			}
		}
		if !isValidSymlink {
			files = append(files, staleSettingsInfo{
				path:          staleTownRootSettings,
				agentType:     "mayor",
				sessionName:   "hq-mayor",
				wrongLocation: true,
				gitStatus:     c.getGitFileStatus(staleTownRootSettings),
				missing:       []string{"should be a symlink to mayor/.claude/settings.json, not a regular file"},
			})
		}
	}

	// Check for CLAUDE.md at town root (~/gt/CLAUDE.md)
	// A NEUTRAL identity anchor is ALLOWED - it just tells agents to run gt prime.
	// Role-specific (Mayor) content is NOT allowed - it pollutes all child workspaces.
	// See priming_check.go for why the neutral anchor is needed.
	townRootCLAUDEmd := filepath.Join(townRoot, "CLAUDE.md")
	if fileExists(townRootCLAUDEmd) {
		if !c.isNeutralIdentityAnchor(townRootCLAUDEmd) {
			files = append(files, staleSettingsInfo{
				path:          townRootCLAUDEmd,
				agentType:     "mayor",
				sessionName:   "hq-mayor",
				wrongLocation: true,
				gitStatus:     c.getGitFileStatus(townRootCLAUDEmd),
				missing:       []string{"contains role-specific content; should be neutral anchor or moved to mayor/CLAUDE.md"},
			})
		}
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
				crewClaudeDir := filepath.Join(crewDir, crewEntry.Name(), ".claude")
				// Skip if .claude is a symlink to the shared parent directory.
				// Crew workers use `.claude -> ../.claude` symlinks so all workers
				// share crew/.claude/settings.json (the correct location).
				if isSymlinkToSharedDir(crewClaudeDir, filepath.Join(crewDir, ".claude")) {
					continue
				}
				crewWrongSettings := filepath.Join(crewClaudeDir, "settings.json")
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
				wrongPaths := []struct {
					claudeDir    string
					settingsPath string
				}{
					{
						filepath.Join(polecatsDir, pcEntry.Name(), ".claude"),
						filepath.Join(polecatsDir, pcEntry.Name(), ".claude", "settings.json"),
					},
					{
						filepath.Join(polecatsDir, pcEntry.Name(), rigName, ".claude"),
						filepath.Join(polecatsDir, pcEntry.Name(), rigName, ".claude", "settings.json"),
					},
				}
				for _, wp := range wrongPaths {
					// Skip if .claude is a symlink to the shared parent directory.
					if isSymlinkToSharedDir(wp.claudeDir, filepath.Join(polecatsDir, ".claude")) {
						continue
					}
					if fileExists(wp.settingsPath) {
						files = append(files, staleSettingsInfo{
							path:          wp.settingsPath,
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
// agentType is used for role-specific validation (autonomous vs interactive).
func (c *ClaudeSettingsCheck) checkSettings(path, agentType string) []string {
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
	// 2. Stop hook with gt decision turn-check
	// 3. gt nudge deacon session-started in SessionStart

	// Check enabledPlugins
	if _, ok := actual["enabledPlugins"]; !ok {
		missing = append(missing, "enabledPlugins")
	}

	// Check hooks
	hooks, ok := actual["hooks"].(map[string]any)
	if !ok {
		return append(missing, "hooks")
	}

	// Check SessionStart hook has deacon nudge
	if !c.hookHasPattern(hooks, "SessionStart", "gt nudge deacon session-started") {
		missing = append(missing, "deacon nudge")
	}

	// Check Stop hook has turn-check with stdin piping (turn enforcement)
	// Must capture stdin and pipe it to turn-check so it can read the session_id (gt-0g7zw4.5)
	if !c.hookHasPattern(hooks, "Stop", "gt decision turn-check") {
		missing = append(missing, "turn-check hook")
	} else if !c.hookHasStdinPiping(hooks, "Stop", "turn-check") {
		missing = append(missing, "turn-check stdin piping")
	}

	// Check UserPromptSubmit hook has bd decision check --inject with stdin piping
	// Must capture stdin and pipe it to bd decision check (gt-0g7zw4.4)
	if !c.hookHasPattern(hooks, "UserPromptSubmit", "bd decision check --inject") {
		missing = append(missing, "decision check hook")
	} else if !c.hookHasBdStdinPiping(hooks, "UserPromptSubmit") {
		missing = append(missing, "bd decision check stdin piping")
	}

	// Check UserPromptSubmit hook has turn-clear with stdin piping (turn enforcement)
	// Must capture stdin and pipe it to turn-clear so it can read the session_id (gt-te4okj)
	if !c.hookHasPattern(hooks, "UserPromptSubmit", "gt decision turn-clear") {
		missing = append(missing, "turn-clear hook")
	} else if !c.hookHasStdinPiping(hooks, "UserPromptSubmit", "turn-clear") {
		missing = append(missing, "turn-clear stdin piping")
	}

	// Note: turn-mark hook is no longer required - marker is set directly in gt decision request
	// (commit 4f1918e8)

	// Check PostToolUse hook has inject drain (queue-based injection pipeline)
	if !c.hookHasPattern(hooks, "PostToolUse", "gt inject drain") {
		missing = append(missing, "inject drain hook")
	}

	// Check SessionStart hook has mail inject (autonomous roles only).
	// Interactive roles (mayor, crew) receive mail via UserPromptSubmit instead.
	if isAutonomousAgentType(agentType) {
		if !c.hookHasPattern(hooks, "SessionStart", "gt mail check --inject") {
			missing = append(missing, "mail inject hook")
		}
	}

	return missing
}

// checkSettingsFreshness compares installed settings hash against the template hash.
// Returns true if the settings match the template (fresh), false if stale.
func (c *ClaudeSettingsCheck) checkSettingsFreshness(path, agentType string) bool {
	// Read installed settings
	installed, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	// Calculate hash of installed settings
	installedHash := fmt.Sprintf("%x", sha256.Sum256(installed))[:8]

	// Get template hash for this agent type
	templateHash := claude.TemplateVersion(claude.RoleTypeFor(agentType))

	return installedHash == templateHash
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

// hookHasStdinPiping checks if a hook command properly pipes stdin to a specific command.
// The pattern should be: _stdin=$(cat) ... && (echo "$_stdin" | gt decision <cmd> ...)
// This is required for hooks that need to read session_id from stdin (gt-te4okj).
func (c *ClaudeSettingsCheck) hookHasStdinPiping(hooks map[string]any, hookName, cmdName string) bool {
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
			if !ok {
				continue
			}
			// Check for the stdin capture pattern AND the piping pattern for this specific command
			if strings.Contains(cmd, "_stdin=$(cat)") &&
				strings.Contains(cmd, fmt.Sprintf("echo \"$_stdin\" | gt decision %s", cmdName)) {
				return true
			}
		}
	}
	return false
}

// hookHasBdStdinPiping checks if a hook command properly pipes stdin to bd decision check.
// The pattern should be: _stdin=$(cat) ... && (echo "$_stdin" | bd decision check ...)
// This is required for hooks that need to read session_id from stdin (gt-0g7zw4.4).
func (c *ClaudeSettingsCheck) hookHasBdStdinPiping(hooks map[string]any, hookName string) bool {
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
			if !ok {
				continue
			}
			// Check for the stdin capture pattern AND the piping pattern for bd decision check
			if strings.Contains(cmd, "_stdin=$(cat)") &&
				strings.Contains(cmd, "echo \"$_stdin\" | bd decision check") {
				return true
			}
		}
	}
	return false
}

// Fix deletes stale settings files and restarts affected agents.
// Files with local modifications are renamed to .bak.<timestamp> instead of being skipped.
func (c *ClaudeSettingsCheck) Fix(ctx *CheckContext) error {
	var errors []string
	var renamed []string
	t := tmux.NewTmux()

	for _, sf := range c.staleSettings {
		wasRenamed := false

		// Files with local modifications get renamed to preserve changes
		if sf.wrongLocation && sf.gitStatus == gitStatusTrackedModified {
			backupPath := fmt.Sprintf("%s.bak.%d", sf.path, time.Now().Unix())
			if err := os.Rename(sf.path, backupPath); err != nil {
				errors = append(errors, fmt.Sprintf("failed to rename %s to backup: %v", sf.path, err))
				continue
			}
			renamed = append(renamed, fmt.Sprintf("%s → %s", sf.path, backupPath))
			wasRenamed = true
		}

		// Delete the stale settings file (skip if already renamed)
		if !wasRenamed {
			if err := os.Remove(sf.path); err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete %s: %v", sf.path, err))
				continue
			}
		}

		// Also delete parent .claude directory if empty
		claudeDir := filepath.Dir(sf.path)
		_ = os.Remove(claudeDir) // Best-effort, will fail if not empty

		// For files in wrong locations, delete and create at correct location
		if sf.wrongLocation {
			mayorDir := filepath.Join(ctx.TownRoot, "mayor")

			// For mayor settings.json at town root, create at mayor/.claude/
			// and symlink from town root so the mayor session (which runs from
			// town root) can find its hooks.
			if sf.agentType == "mayor" && strings.HasSuffix(claudeDir, ".claude") && !strings.Contains(sf.path, "/mayor/") {
				if err := os.MkdirAll(mayorDir, 0755); err == nil {
					_ = claude.EnsureSettingsForRole(mayorDir, "mayor")
				}
				// Create symlink from town root to mayor settings
				townClaudeDir := filepath.Join(ctx.TownRoot, ".claude")
				symlinkPath := filepath.Join(townClaudeDir, "settings.json")
				if err := os.MkdirAll(townClaudeDir, 0755); err == nil {
					relTarget := filepath.Join("..", "mayor", ".claude", "settings.json")
					_ = os.Symlink(relTarget, symlinkPath)
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
					// Cycle the agent by killing and letting gt up restart it.
					// Use KillSessionWithProcesses to ensure all descendant processes are killed.
					_ = t.KillSessionWithProcesses(sf.sessionName)
				}
			}
		}
	}

	// Report renamed files as info
	if len(renamed) > 0 {
		fmt.Printf("  Renamed files with local modifications (backups preserved):\n")
		for _, r := range renamed {
			fmt.Printf("    %s\n", r)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// isSymlinkToSharedDir checks if claudeDir is a symlink that resolves to sharedDir.
// This prevents gt doctor --fix from deleting shared settings files that are
// correctly symlinked from worker directories (e.g., crew/<name>/.claude -> ../.claude).
func isSymlinkToSharedDir(claudeDir, sharedDir string) bool {
	fi, err := os.Lstat(claudeDir)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(claudeDir)
	if err != nil {
		return false
	}
	sharedResolved, err := filepath.EvalSymlinks(sharedDir)
	if err != nil {
		// If the shared dir doesn't exist, compare against the raw path
		sharedResolved = sharedDir
	}
	return resolved == sharedResolved
}

// isNeutralIdentityAnchor checks if a CLAUDE.md file is a neutral identity anchor.
// A neutral anchor tells agents to run gt prime without containing role-specific content.
// This is ALLOWED at town root per priming_check.go rationale.
func (c *ClaudeSettingsCheck) isNeutralIdentityAnchor(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(data)

	// Must contain the gt prime instruction
	if !strings.Contains(content, "gt prime") {
		return false
	}

	// Must contain the identity delegation message
	if !strings.Contains(content, "identity and role are determined by") &&
		!strings.Contains(content, "Your identity and role are determined") {
		return false
	}

	// Must NOT contain Mayor-specific instructions (role pollution)
	mayorPatterns := []string{
		"Mayor coordinates",
		"mayor session",
		"hq-mayor",
		"As Mayor",
		"Mayor role",
	}
	for _, pattern := range mayorPatterns {
		if strings.Contains(content, pattern) {
			return false
		}
	}

	return true
}

// isAutonomousAgentType returns true for agent types that use autonomous settings.
func isAutonomousAgentType(agentType string) bool {
	switch agentType {
	case "polecat", "witness", "refinery", "deacon", "boot":
		return true
	default:
		return false
	}
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
