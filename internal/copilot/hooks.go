// Package copilot provides GitHub Copilot CLI integration for Gas Town.
package copilot

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/claude"
)

//go:embed plugin/hooks-autonomous.json
var hooksAutonomousJSON []byte

//go:embed plugin/hooks-interactive.json
var hooksInteractiveJSON []byte

//go:embed plugin/gastown-pretool-guard.sh
var guardScript []byte

// EnsureHooksAt provisions the Copilot CLI hooks directory.
//
// Lifecycle hooks (sessionStart, userPromptSubmitted, sessionEnd) are provided
// by the gastown Copilot CLI plugin (plugins/copilot-cli/). This function only
// installs the per-project guard script and a stub hooks JSON marker file.
//
// Install the plugin with: copilot plugin install ./plugins/copilot-cli
func EnsureHooksAt(workDir, role, hooksDir, hooksFile string) error {
	if hooksDir == "" || hooksFile == "" {
		return nil
	}

	targetDir := filepath.Join(workDir, hooksDir)

	// Select stub template based on role type (both are empty hooks now)
	var templateData []byte
	if claude.RoleTypeFor(role) == claude.Autonomous {
		templateData = hooksAutonomousJSON
	} else {
		templateData = hooksInteractiveJSON
	}

	// Write hooks JSON stub (marker file, no active hooks — plugin handles them)
	hooksPath := filepath.Join(targetDir, hooksFile)
	if _, err := os.Stat(hooksPath); os.IsNotExist(err) {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("creating copilot hooks directory: %w", err)
		}
		if err := os.WriteFile(hooksPath, templateData, 0644); err != nil {
			return fmt.Errorf("writing copilot hooks: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("checking copilot hooks file: %w", err)
	}

	// Write guard script (called by plugin's preToolUse hook)
	guardPath := filepath.Join(targetDir, "gastown-pretool-guard.sh")
	if _, err := os.Stat(guardPath); os.IsNotExist(err) {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("creating copilot hooks directory: %w", err)
		}
		if err := os.WriteFile(guardPath, guardScript, 0755); err != nil {
			return fmt.Errorf("writing copilot guard script: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("checking copilot guard script: %w", err)
	}

	return nil
}
