// Package copilot provides GitHub Copilot CLI integration for Gas Town.
package copilot

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed plugin/gastown-instructions.md
var pluginFS embed.FS

// EnsureSettingsAt ensures the Gas Town custom instructions file exists for Copilot.
// If the file already exists, it's left unchanged.
// workDir is the agent's working directory where instructions are provisioned.
// hooksDir is the directory within workDir (e.g., ".copilot").
// hooksFile is the filename (e.g., "copilot-instructions.md").
func EnsureSettingsAt(workDir, hooksDir, hooksFile string) error {
	if hooksDir == "" || hooksFile == "" {
		return nil
	}

	settingsPath := filepath.Join(workDir, hooksDir, hooksFile)
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking copilot instructions file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("creating copilot settings directory: %w", err)
	}

	content, err := pluginFS.ReadFile("plugin/gastown-instructions.md")
	if err != nil {
		return fmt.Errorf("reading copilot instructions template: %w", err)
	}

	if err := os.WriteFile(settingsPath, content, 0644); err != nil {
		return fmt.Errorf("writing copilot instructions: %w", err)
	}

	return nil
}
