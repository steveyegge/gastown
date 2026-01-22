package hooks

import (
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/opencode"
)

// opencodeProvider implements Provider for OpenCode CLI.
// It uses the existing opencode package to install the Gas Town plugin.
type opencodeProvider struct{}

func init() {
	Register(&opencodeProvider{})
}

// Name returns "opencode".
func (p *opencodeProvider) Name() string {
	return "opencode"
}

// EnsureHooks installs the OpenCode plugin in the given directory.
// Uses the existing opencode.EnsurePluginAt function to install gastown.js.
func (p *opencodeProvider) EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error {
	// Determine plugin directory and file from config, or use defaults
	pluginDir := ".opencode/plugin"
	pluginFile := "gastown.js"
	if hooksConfig != nil {
		if hooksConfig.Dir != "" {
			pluginDir = hooksConfig.Dir
		}
		if hooksConfig.SettingsFile != "" {
			pluginFile = hooksConfig.SettingsFile
		}
	}

	// Use the existing opencode package to install the plugin
	return opencode.EnsurePluginAt(workDir, pluginDir, pluginFile)
}

// SupportsHooks returns true because OpenCode has native hook/plugin support.
func (p *opencodeProvider) SupportsHooks() bool {
	return true
}

// GetHooksFallback returns nil because OpenCode supports native hooks.
func (p *opencodeProvider) GetHooksFallback(role string) []string {
	return nil
}
