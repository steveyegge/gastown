// Package cursor provides Cursor agent configuration management.
package cursor

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/constants"
)

//go:embed config/*.json
var configFS embed.FS

// RoleType indicates whether a role is autonomous or interactive.
type RoleType string

const (
	// Autonomous roles (polecat, witness, refinery) need mail in sessionStart
	// because they may be triggered externally without user input.
	Autonomous RoleType = "autonomous"

	// Interactive roles (mayor, crew) wait for user input, so beforeSubmitPrompt
	// handles mail injection.
	Interactive RoleType = "interactive"
)

// RoleTypeFor returns the RoleType for a given role name.
func RoleTypeFor(role string) RoleType {
	switch role {
	case constants.RolePolecat, constants.RoleWitness, constants.RoleRefinery, constants.RoleDeacon, "boot":
		return Autonomous
	default:
		return Interactive
	}
}

// EnsureHooksAt ensures a hooks.json file exists at a custom directory/file.
// Cursor has no --settings flag, so hooks are installed in workDir
// (the agent's working directory), not a separate settingsDir.
// If the file already exists, it's left unchanged.
func EnsureHooksAt(workDir string, roleType RoleType, hooksDir, hooksFile string) error {
	cursorDir := filepath.Join(workDir, hooksDir)
	hooksPath := filepath.Join(cursorDir, hooksFile)

	if _, err := os.Stat(hooksPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}
	var templateName string
	switch roleType {
	case Autonomous:
		templateName = "config/hooks-autonomous.json"
	default:
		templateName = "config/hooks-interactive.json"
	}
	content, err := configFS.ReadFile(templateName)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", templateName, err)
	}
	if err := os.WriteFile(hooksPath, content, 0600); err != nil {
		return fmt.Errorf("writing hooks: %w", err)
	}
	return nil
}

// EnsureHooksForRoleAt is a convenience function that combines RoleTypeFor and EnsureHooksAt.
func EnsureHooksForRoleAt(workDir, role, hooksDir, hooksFile string) error {
	return EnsureHooksAt(workDir, RoleTypeFor(role), hooksDir, hooksFile)
}
