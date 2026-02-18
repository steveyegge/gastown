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

// EnsurePluginAt ensures the Gas Town OpenCode plugin exists.
// If the file already exists, it's left unchanged.
func EnsurePluginAt(workDir, pluginDir, pluginFile string) error {
	if pluginDir == "" || pluginFile == "" {
		return nil
	}

	pluginPath := filepath.Join(workDir, pluginDir, pluginFile)
	if _, err := os.Stat(pluginPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(pluginPath), 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		return fmt.Errorf("reading plugin template: %w", err)
	}

	if err := os.WriteFile(pluginPath, content, 0644); err != nil {
		return fmt.Errorf("writing plugin: %w", err)
	}

	return nil
}
