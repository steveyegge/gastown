// Package opencode provides OpenCode plugin management.
package opencode

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed plugin/gastown.js
var pluginFS embed.FS

// EnsurePluginAt ensures the Gas Town OpenCode plugin is up to date.
// Always writes the latest embedded version to keep plugins current.
func EnsurePluginAt(workDir, pluginDir, pluginFile string) error {
	if pluginDir == "" || pluginFile == "" {
		return nil
	}

	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		return fmt.Errorf("reading plugin template: %w", err)
	}

	pluginPath := filepath.Join(workDir, pluginDir, pluginFile)

	// Skip write if content is already current.
	if existing, err := os.ReadFile(pluginPath); err == nil {
		if string(existing) == string(content) {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(pluginPath), 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	if err := os.WriteFile(pluginPath, content, 0644); err != nil {
		return fmt.Errorf("writing plugin: %w", err)
	}

	return nil
}
