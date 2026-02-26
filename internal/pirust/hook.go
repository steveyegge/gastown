// Package pirust provides Pi Agent Rust hook management.
// Separate from the pi package because pi-rust uses different event names
// (startup/agent_end vs session_start/session_shutdown) and injects context
// via tmux send-keys rather than system prompt return values.
package pirust

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed gastown-hooks.js
var hookFS embed.FS

// EnsureHookAt ensures the Gas Town Pi-Rust extension hook exists.
// If the file already exists, it's left unchanged.
func EnsureHookAt(workDir, hooksDir, hooksFile string) error {
	if hooksDir == "" || hooksFile == "" {
		return nil
	}

	hookPath := filepath.Join(workDir, hooksDir, hooksFile)
	if _, err := os.Stat(hookPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(hookPath), 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	content, err := hookFS.ReadFile("gastown-hooks.js")
	if err != nil {
		return fmt.Errorf("reading hook template: %w", err)
	}

	if err := os.WriteFile(hookPath, content, 0644); err != nil {
		return fmt.Errorf("writing hook: %w", err)
	}

	return nil
}
