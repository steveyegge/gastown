package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/copilot"
)

// CopilotTrustConfig controls how Copilot trusted_folders are updated.
type CopilotTrustConfig struct {
	Role          string
	TownRoot      string
	RigPath       string
	WorkDir       string
	AgentOverride string
}

// EnsureCopilotTrustedFolder ensures Copilot trusts the session's working directory.
func EnsureCopilotTrustedFolder(cfg CopilotTrustConfig) error {
	if cfg.WorkDir == "" {
		return nil
	}

	rc, err := resolveRuntimeForCopilot(cfg)
	if err != nil {
		return err
	}
	if !isCopilotRuntime(rc) {
		return nil
	}

	if _, err := copilot.EnsureTrustedFolder(cfg.WorkDir); err != nil {
		return fmt.Errorf("updating copilot trusted_folders: %w", err)
	}
	return nil
}

func resolveRuntimeForCopilot(cfg CopilotTrustConfig) (*RuntimeConfig, error) {
	if cfg.AgentOverride != "" {
		rc, _, err := ResolveAgentConfigWithOverride(cfg.TownRoot, cfg.RigPath, cfg.AgentOverride)
		if err != nil {
			return nil, fmt.Errorf("resolving agent override: %w", err)
		}
		return rc, nil
	}
	if cfg.Role != "" {
		return ResolveRoleAgentConfig(cfg.Role, cfg.TownRoot, cfg.RigPath), nil
	}
	return ResolveAgentConfig(cfg.TownRoot, cfg.RigPath), nil
}

func isCopilotRuntime(rc *RuntimeConfig) bool {
	if rc == nil {
		return false
	}
	if strings.EqualFold(rc.Provider, string(AgentCopilot)) {
		return true
	}
	if rc.Command == "" {
		return false
	}
	return strings.EqualFold(filepath.Base(rc.Command), string(AgentCopilot))
}
