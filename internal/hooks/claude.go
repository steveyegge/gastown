package hooks

import (
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
)

// claudeProvider implements Provider for Claude Code.
type claudeProvider struct{}

func init() {
	Register(&claudeProvider{})
}

func (p *claudeProvider) Name() string {
	return "claude"
}

func (p *claudeProvider) EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error {
	dir := hooksConfig.Dir
	if dir == "" {
		dir = ".claude"
	}
	file := hooksConfig.SettingsFile
	if file == "" {
		file = "settings.json"
	}
	return claude.EnsureSettingsForRoleAt(workDir, role, dir, file)
}
