// Package claude provides Claude Code configuration management.
package claude

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed config/*.json config/*.md
var configFS embed.FS

// RoleType indicates whether a role is autonomous or interactive.
type RoleType string

const (
	// Autonomous roles (polecat, witness, refinery) need mail in SessionStart
	// because they may be triggered externally without user input.
	Autonomous RoleType = "autonomous"

	// Interactive roles (mayor, crew) wait for user input, so UserPromptSubmit
	// handles mail injection.
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
