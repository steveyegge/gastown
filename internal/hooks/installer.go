// Package hooks provides a generic hook/settings installer for all agent runtimes.
//
// Instead of per-agent packages (claude/, gemini/, cursor/, etc.) each containing
// near-identical boilerplate, this package embeds all agent templates and provides
// a single generic installer that reads template metadata from AgentPresetInfo.
package hooks

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/hookutil"
)

//go:embed templates/*
var templateFS embed.FS

// InstallForRole provisions hook/settings files for an agent based on its preset config.
// settingsDir is the gastown-managed parent (used by agents with --settings flag).
// workDir is the agent's working directory.
// role is the Gas Town role (e.g., "polecat", "crew", "witness").
// hooksDir and hooksFile come from the preset's HooksDir and HooksSettingsFile.
// provider is the preset's HooksProvider (e.g., "claude", "gemini").
//
// Template resolution:
//   - Role-aware agents (have both autonomous and interactive templates):
//     templates/<provider>/settings-autonomous.json + settings-interactive.json
//     or templates/<provider>/hooks-autonomous.json + hooks-interactive.json
//   - Role-agnostic agents (single template): templates/<provider>/<hooksFile>
//
// The install directory is settingsDir for agents that support --settings (useSettingsDir=true),
// or workDir for all others.
func InstallForRole(provider, settingsDir, workDir, role, hooksDir, hooksFile string, useSettingsDir bool) error {
	if provider == "" || hooksDir == "" || hooksFile == "" {
		return nil
	}

	// Determine install root
	installDir := workDir
	if useSettingsDir {
		installDir = settingsDir
	}

	targetPath := filepath.Join(installDir, hooksDir, hooksFile)

	// Don't overwrite existing files
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	// Try role-aware templates first (autonomous/interactive variants)
	content, err := resolveTemplate(provider, hooksFile, role)
	if err != nil {
		return fmt.Errorf("resolving template for %s: %w", provider, err)
	}

	// Use restrictive permissions for settings that may contain role instructions
	perm := os.FileMode(0644)
	if isSettingsFile(hooksFile) {
		perm = 0600
	}

	if err := os.WriteFile(targetPath, content, perm); err != nil {
		return fmt.Errorf("writing hooks file: %w", err)
	}

	return nil
}

// resolveTemplate finds the right template for a provider+role combination.
func resolveTemplate(provider, hooksFile, role string) ([]byte, error) {
	// Determine role type
	autonomous := hookutil.IsAutonomousRole(role)

	// Try role-aware naming conventions
	if autonomous {
		for _, pattern := range roleAwarePatterns("autonomous", hooksFile) {
			path := fmt.Sprintf("templates/%s/%s", provider, pattern)
			if content, err := templateFS.ReadFile(path); err == nil {
				return content, nil
			}
		}
	} else {
		for _, pattern := range roleAwarePatterns("interactive", hooksFile) {
			path := fmt.Sprintf("templates/%s/%s", provider, pattern)
			if content, err := templateFS.ReadFile(path); err == nil {
				return content, nil
			}
		}
	}

	// Fall back to single template (role-agnostic agents)
	path := fmt.Sprintf("templates/%s/%s", provider, hooksFile)
	content, err := templateFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no template found for provider %q file %q: %w", provider, hooksFile, err)
	}
	return content, nil
}

// roleAwarePatterns generates candidate template filenames for role-aware agents.
// Given roleType "autonomous" and hooksFile "settings.json", it tries:
//   - settings-autonomous.json
//   - hooks-autonomous.json
func roleAwarePatterns(roleType, hooksFile string) []string {
	ext := filepath.Ext(hooksFile)
	base := hooksFile[:len(hooksFile)-len(ext)]

	return []string{
		base + "-" + roleType + ext,  // settings-autonomous.json
		"hooks-" + roleType + ext,    // hooks-autonomous.json
		"settings-" + roleType + ext, // settings-autonomous.json (fallback)
	}
}

// isSettingsFile returns true for files that may contain sensitive role config.
func isSettingsFile(name string) bool {
	return filepath.Ext(name) == ".json"
}
