// Package claude provides Claude Code configuration management.
package claude

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed config/*.json
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
	case "polecat", "witness", "refinery":
		return Autonomous
	default:
		return Interactive
	}
}

// EnsureSettings ensures .claude/settings.json exists in the given directory.
// If the file doesn't exist, it copies the appropriate template based on role type.
// If the file already exists, it's left unchanged.
func EnsureSettings(workDir string, roleType RoleType) error {
	claudeDir := filepath.Join(workDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// If settings already exist, don't overwrite
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}

	// Create .claude directory if needed
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
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
