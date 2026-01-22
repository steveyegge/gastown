// Package hooks provides a pluggable hook system for different LLM CLI tools.
// It defines the Provider interface that allows Gas Town to work with arbitrary
// CLI tools (Claude, OpenCode, Kiro, etc.) in a provider-agnostic way.
package hooks

import (
	"sync"

	"github.com/steveyegge/gastown/internal/config"
)

// Provider defines the interface for LLM CLI tool hook providers.
// Each provider knows how to install and configure hooks for its CLI tool.
type Provider interface {
	// Name returns the provider identifier (e.g., "claude", "opencode", "kiro").
	Name() string

	// EnsureHooks installs/configures hooks for this provider in the given directory.
	// workDir is the directory where hooks should be installed (e.g., polecat worktree).
	// role is the Gas Town role (e.g., "polecat", "witness", "crew").
	// config contains provider-specific hook settings from RuntimeHooksConfig.
	EnsureHooks(workDir, role string, hooksConfig *config.RuntimeHooksConfig) error

	// SupportsHooks returns true if the provider has native hook support.
	// Providers without native hooks use tmux-based fallback commands.
	SupportsHooks() bool

	// GetHooksFallback returns tmux commands to emulate hooks for providers
	// that don't support native hooks. These commands are injected into the
	// tmux session to provide similar functionality.
	// Returns nil if the provider supports native hooks.
	GetHooksFallback(role string) []string
}

// Registry holds all registered providers with thread-safe access.
var (
	providers   = make(map[string]Provider)
	providersMu sync.RWMutex
)

// Register adds a provider to the registry.
// Providers typically call this in their init() function.
// If a provider with the same name already exists, it is replaced.
func Register(p Provider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[p.Name()] = p
}

// Get returns a provider by name, or nil if not found.
func Get(name string) Provider {
	providersMu.RLock()
	defer providersMu.RUnlock()
	return providers[name]
}

// List returns the names of all registered providers.
func List() []string {
	providersMu.RLock()
	defer providersMu.RUnlock()
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// ResetForTesting clears all registered providers.
// This is intended for use in tests only to ensure test isolation.
func ResetForTesting() {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers = make(map[string]Provider)
}
