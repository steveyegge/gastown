package hooks

import "github.com/steveyegge/gastown/internal/config"

// noneProvider implements Provider for tools without hook support.
// It's a no-op provider that satisfies the interface but does nothing.
type noneProvider struct{}

func init() {
	Register(&noneProvider{})
}

func (p *noneProvider) Name() string {
	return "none"
}

func (p *noneProvider) EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error {
	// No-op: this provider is for tools that don't support hooks.
	return nil
}
