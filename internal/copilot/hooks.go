// Package copilot provides GitHub Copilot CLI integration for Gas Town.
package copilot

import (
	"bytes"
	_ "embed"
	"encoding/json"
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

// hooksConfig is the top-level structure for Copilot CLI hooks.json files.
type hooksConfig struct {
	Version        int                    `json:"version"`
	GastownManaged bool                   `json:"x-gastown-managed"`
	Hooks          map[string][]hookEntry `json:"hooks"`
}

// hookEntry represents a single hook entry in the hooks configuration.
type hookEntry struct {
	Type       string `json:"type"`
	Bash       string `json:"bash"`
	TimeoutSec int    `json:"timeoutSec"`
}

// EnsureHooksAt provisions the Copilot CLI lifecycle hooks configuration.
// It creates .github/hooks/gastown.json and .github/hooks/gastown-pretool-guard.sh
// in the agent's working directory. Files are not overwritten if they already exist.
func EnsureHooksAt(workDir, role, hooksDir, hooksFile string) error {
	if hooksDir == "" || hooksFile == "" {
		return nil
	}

	targetDir := filepath.Join(workDir, hooksDir)

	// Select template based on role type
	var templateData []byte
	if claude.RoleTypeFor(role) == claude.Autonomous {
		templateData = hooksAutonomousJSON
	} else {
		templateData = hooksInteractiveJSON
	}

	// Template the guard script path using the actual hooksDir
	guardRef := filepath.ToSlash(filepath.Join(hooksDir, "gastown-pretool-guard.sh"))
	templateData = bytes.Replace(templateData,
		[]byte(".github/hooks/gastown-pretool-guard.sh"), []byte(guardRef), -1)

	// Validate the template is valid JSON
	var config hooksConfig
	if err := json.Unmarshal(templateData, &config); err != nil {
		return fmt.Errorf("invalid hooks template: %w", err)
	}

	// Write hooks JSON file
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

	// Write guard script
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
