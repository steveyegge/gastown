package hooks

import (
	"github.com/steveyegge/gastown/internal/config"
)

// noneProvider is a no-op provider for CLI tools that don't support hooks.
// It provides tmux-based fallback commands to emulate hook behavior.
type noneProvider struct{}

func init() {
	Register(&noneProvider{})
}

// Name returns "none".
func (p *noneProvider) Name() string {
	return "none"
}

// EnsureHooks is a no-op for providers without hook support.
func (p *noneProvider) EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error {
	return nil
}

// SupportsHooks returns false because this provider has no native hook support.
func (p *noneProvider) SupportsHooks() bool {
	return false
}

// GetHooksFallback returns tmux commands to inject Gas Town context.
// These commands are executed via tmux send-keys before starting the CLI tool.
func (p *noneProvider) GetHooksFallback(role string) []string {
	// Basic initialization commands that work for any CLI tool
	cmds := []string{
		"gt prime",
	}

	// Autonomous roles need mail injection at startup
	switch role {
	case "polecat", "witness", "refinery", "deacon":
		cmds = append(cmds, "gt mail check --inject")
	}

	return cmds
}
