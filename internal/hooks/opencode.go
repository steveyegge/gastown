package hooks

import (
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/opencode"
)

// opencodeProvider implements Provider for OpenCode.
type opencodeProvider struct{}

func init() {
	Register(&opencodeProvider{})
}

func (p *opencodeProvider) Name() string {
	return "opencode"
}

func (p *opencodeProvider) EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error {
	// OpenCode doesn't differentiate by role - same plugin for all roles.
	return opencode.EnsurePluginAt(workDir, hooksConfig.Dir, hooksConfig.SettingsFile)
}
