// Package claude provides Claude Code configuration management.
package claude

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed config/*.json config/*.md
var configFS embed.FS

// RoleType indicates the settings template to use for a role.
type RoleType string

const (
	// Autonomous roles (polecat, witness, refinery, deacon, boot) need mail in SessionStart
	// because they may be triggered externally without user input.
	// They do NOT have decision hooks - they work autonomously without human escalation.
	Autonomous RoleType = "autonomous"

	// Interactive roles (mayor, crew) wait for user input, so UserPromptSubmit
	// handles mail injection. They have decision hooks for human-in-the-loop.
	Interactive RoleType = "interactive"
)

// RoleTypeFor returns the RoleType for a given role name.
func RoleTypeFor(role string) RoleType {
	switch role {
	case "polecat", "witness", "refinery", "deacon", "boot":
		return Autonomous
	default:
		return Interactive
	}
}

// TemplateVersion returns a short hash of the embedded template content.
// Used by gt doctor to detect stale installed settings.
func TemplateVersion(roleType RoleType) string {
	var templateName string
	switch roleType {
	case Autonomous:
		templateName = "config/settings-autonomous.json"
	default:
		templateName = "config/settings-interactive.json"
	}
	content, err := configFS.ReadFile(templateName)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)[:8]
}

// TemplateContent returns the raw content of the embedded template.
// Used for comparing installed settings against templates.
func TemplateContent(roleType RoleType) ([]byte, error) {
	var templateName string
	switch roleType {
	case Autonomous:
		templateName = "config/settings-autonomous.json"
	default:
		templateName = "config/settings-interactive.json"
	}
	return configFS.ReadFile(templateName)
}

// EnsureSettings ensures .claude/settings.json exists in the given directory.
// For worktrees, we use sparse checkout to exclude source repo's .claude/ directory,
// so our settings.json is the only one Claude Code sees.
func EnsureSettings(workDir string, roleType RoleType) error {
	return EnsureSettingsAt(workDir, roleType, ".claude", "settings.json")
}

// EnsureSettingsAt ensures a settings file exists at a custom directory/file.
// If the file doesn't exist, it copies the appropriate template based on role type.
// If the file already exists, it's left unchanged.
func EnsureSettingsAt(workDir string, roleType RoleType, settingsDir, settingsFile string) error {
	claudeDir := filepath.Join(workDir, settingsDir)
	settingsPath := filepath.Join(claudeDir, settingsFile)

	// If settings already exist, don't overwrite
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}

	// Create settings directory if needed
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}

	// Select template based on role type
	var templateName string
	switch roleType {
	case Autonomous:
		templateName = "config/settings-autonomous.json"
	default:
		templateName = "config/settings-interactive.json"
	}

	// Read template
	content, err := configFS.ReadFile(templateName)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", templateName, err)
	}

	// Write settings file
	if err := os.WriteFile(settingsPath, content, 0600); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	return nil
}

// EnsureSettingsForRole is a convenience function that combines RoleTypeFor and EnsureSettings.
func EnsureSettingsForRole(workDir, role string) error {
	return EnsureSettings(workDir, RoleTypeFor(role))
}

// EnsureSettingsForRoleAt is a convenience function that combines RoleTypeFor and EnsureSettingsAt.
func EnsureSettingsForRoleAt(workDir, role, settingsDir, settingsFile string) error {
	return EnsureSettingsAt(workDir, RoleTypeFor(role), settingsDir, settingsFile)
}

// ProvisionFileAfterFail copies FILE_AFTER_FAIL.md to a crew workspace.
// This template documents the "Fail then File" principle for crew workers.
// If the file already exists, it's left unchanged.
func ProvisionFileAfterFail(workDir string) error {
	destPath := filepath.Join(workDir, "FILE_AFTER_FAIL.md")

	// If file already exists, don't overwrite
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}

	// Read template from embedded filesystem
	content, err := configFS.ReadFile("config/FILE_AFTER_FAIL.md")
	if err != nil {
		return fmt.Errorf("reading FILE_AFTER_FAIL.md template: %w", err)
	}

	// Write the file
	if err := os.WriteFile(destPath, content, 0644); err != nil {
		return fmt.Errorf("writing FILE_AFTER_FAIL.md: %w", err)
	}

	return nil
}

// EnsureMCPConfig ensures .mcp.json exists in the given directory.
// This configures MCP servers (like Playwright) for Claude Code.
// If the file already exists, it's left unchanged.
func EnsureMCPConfig(workDir string) error {
	return EnsureMCPConfigAt(workDir, ".mcp.json")
}

// EnsureMCPConfigAt ensures .mcp.json exists at a custom path.
// If the file already exists, it's left unchanged.
func EnsureMCPConfigAt(workDir, mcpFile string) error {
	mcpPath := filepath.Join(workDir, mcpFile)

	// If MCP config already exists, don't overwrite
	if _, err := os.Stat(mcpPath); err == nil {
		return nil
	}

	// Read MCP template
	content, err := configFS.ReadFile("config/mcp.json")
	if err != nil {
		return fmt.Errorf("reading mcp.json template: %w", err)
	}

	// Write MCP config file
	if err := os.WriteFile(mcpPath, content, 0644); err != nil {
		return fmt.Errorf("writing .mcp.json: %w", err)
	}

	return nil
}

// EnsureSkills sets up skill symlinks in .claude/skills/ directory.
// It looks for skill definitions in rigRoot/skill-library/ and creates
// symlinks to each SKILL.md file. This enables slash commands like /handoff.
// The function is idempotent - it won't create duplicate symlinks.
func EnsureSkills(workDir, rigRoot string) error {
	skillLibrary := filepath.Join(rigRoot, "skill-library")
	skillsDir := filepath.Join(workDir, ".claude", "skills")

	// Check if skill-library exists
	entries, err := os.ReadDir(skillLibrary)
	if os.IsNotExist(err) {
		// No skill-library - nothing to do
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading skill-library: %w", err)
	}

	// Create skills directory if needed
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("creating skills directory: %w", err)
	}

	// Calculate relative path from skills dir to skill-library
	// From: workDir/.claude/skills/
	// To: rigRoot/skill-library/
	relPath, err := filepath.Rel(skillsDir, skillLibrary)
	if err != nil {
		return fmt.Errorf("calculating relative path: %w", err)
	}

	// Create symlinks for each skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillMD := filepath.Join(skillLibrary, skillName, "SKILL.md")

		// Check if SKILL.md exists
		if _, err := os.Stat(skillMD); os.IsNotExist(err) {
			continue
		}

		symlinkPath := filepath.Join(skillsDir, skillName)

		// Skip if symlink already exists
		if _, err := os.Lstat(symlinkPath); err == nil {
			continue
		}

		// Create relative symlink
		target := filepath.Join(relPath, skillName, "SKILL.md")
		if err := os.Symlink(target, symlinkPath); err != nil {
			return fmt.Errorf("creating symlink for skill %s: %w", skillName, err)
		}
	}

	return nil
}

// EnsureSettingsForAccount ensures settings.json exists for a specific account.
// If accountConfigDir is provided, settings are installed there (per-account).
// If accountConfigDir is empty, falls back to workDir (per-workspace).
// accountConfigDir should be the account's CLAUDE_CONFIG_DIR path (e.g., ~/.claude-accounts/work).
func EnsureSettingsForAccount(workDir, role, accountConfigDir string) error {
	roleType := RoleTypeFor(role)

	// If no account config dir, use workspace settings (legacy behavior)
	if accountConfigDir == "" {
		return EnsureSettings(workDir, roleType)
	}

	// Install settings into account config directory
	settingsPath := filepath.Join(accountConfigDir, "settings.json")

	// If settings already exist, don't overwrite
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}

	// Create account config directory if needed
	if err := os.MkdirAll(accountConfigDir, 0755); err != nil {
		return fmt.Errorf("creating account config directory: %w", err)
	}

	// Select template based on role type
	var templateName string
	switch roleType {
	case Autonomous:
		templateName = "config/settings-autonomous.json"
	default:
		templateName = "config/settings-interactive.json"
	}

	// Read template
	content, err := configFS.ReadFile(templateName)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", templateName, err)
	}

	// Write settings file
	if err := os.WriteFile(settingsPath, content, 0600); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	return nil
}
