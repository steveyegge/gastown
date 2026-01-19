// Package hooks provides a generic interface for LLM tool hook providers.
//
// Hook providers install configuration files that allow runtime tools
// (like Claude Code or OpenCode) to execute Gas Town commands at various
// lifecycle events (session start, prompt submit, etc.).
package hooks

import "github.com/steveyegge/gastown/internal/config"

// Provider is the interface for LLM tool hook providers.
type Provider interface {
	// Name returns the provider identifier (e.g., "claude", "opencode", "none").
	Name() string

	// EnsureHooks installs hook configuration for the given role.
	// workDir is the workspace directory.
	// role is the agent role (e.g., "polecat", "witness", "mayor").
	// config contains provider-specific paths (Dir, SettingsFile).
	// Returns nil if hooks are successfully installed or already exist.
	EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error
}

// registry holds registered providers.
var registry = make(map[string]Provider)

// Register adds a provider to the registry.
// This should be called from provider init() functions.
func Register(p Provider) {
	registry[p.Name()] = p
}

// Get returns a provider by name, or nil if not found.
func Get(name string) Provider {
	return registry[name]
}

// Names returns all registered provider names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
