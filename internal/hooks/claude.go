package hooks

import (
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
)

// claudeProvider implements Provider for Claude Code.
// It uses the existing claude package to install hooks.
type claudeProvider struct{}

func init() {
	Register(&claudeProvider{})
}

// Name returns "claude".
func (p *claudeProvider) Name() string {
	return "claude"
}

// EnsureHooks installs Claude Code hooks in the given directory.
// Uses the existing claude.EnsureSettingsAt function to install settings.json.
func (p *claudeProvider) EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error {
	// Determine settings directory and file from config, or use defaults
	settingsDir := ".claude"
	settingsFile := "settings.json"
	if hooksConfig != nil {
		if hooksConfig.Dir != "" {
			settingsDir = hooksConfig.Dir
		}
		if hooksConfig.SettingsFile != "" {
			settingsFile = hooksConfig.SettingsFile
		}
	}

	// Use the existing claude package to install the appropriate settings
	return claude.EnsureSettingsForRoleAt(workDir, role, settingsDir, settingsFile)
}

// SupportsHooks returns true because Claude Code has native hook support.
func (p *claudeProvider) SupportsHooks() bool {
	return true
}

// GetHooksFallback returns nil because Claude Code supports native hooks.
func (p *claudeProvider) GetHooksFallback(role string) []string {
	return nil
}
