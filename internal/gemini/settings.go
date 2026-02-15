// Package gemini provides Gemini CLI configuration management.
package gemini

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

	// Interactive roles (mayor, crew) wait for user input, so BeforeAgent
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

// EnsureSettingsAt ensures a settings file exists at a custom directory/file.
// Gemini CLI has no --settings flag, so settings are installed in workDir
// (the agent's working directory), not a separate settingsDir.
// If the file doesn't exist, it copies the appropriate template based on role type.
// If the file already exists, it's left unchanged.
func EnsureSettingsAt(workDir string, roleType RoleType, settingsDir, settingsFile string) error {
	geminiDir := filepath.Join(workDir, settingsDir)
	settingsPath := filepath.Join(geminiDir, settingsFile)

	// If settings already exist, don't overwrite
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}

	// Create settings directory if needed
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
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

// EnsureSettingsForRoleAt is a convenience function that combines RoleTypeFor and EnsureSettingsAt.
func EnsureSettingsForRoleAt(workDir, role, settingsDir, settingsFile string) error {
	return EnsureSettingsAt(workDir, RoleTypeFor(role), settingsDir, settingsFile)
}
