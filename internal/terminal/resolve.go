package terminal

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/tmux"
)

// ResolveBackend returns the appropriate Backend for the given agent.
// Resolution order: Coop (if backend=coop) → SSH (if backend=k8s) → local tmux.
//
// The agentID follows the standard format: "rig/polecat" or "rig/crew/name".
// Backend detection checks the agent bead for a "backend" field set by
// the K8s pod manager or Coop sidecar deployment.
func ResolveBackend(agentID string) Backend {
	// Check if agent has Coop backend metadata
	coopCfg, err := resolveCoopConfig(agentID)
	if err == nil && coopCfg != nil {
		b := NewCoopBackend(coopCfg.CoopConfig)
		b.AddSession("claude", coopCfg.baseURL)
		return b
	}

	// Check if agent has K8s/SSH backend metadata
	sshCfg, err := resolveSSHConfig(agentID)
	if err == nil && sshCfg != nil {
		return NewSSHBackend(*sshCfg)
	}

	// Default: local tmux
	return NewTmuxBackend(tmux.NewTmux())
}

// LocalBackend returns a TmuxBackend for local tmux operations.
// Use this when you know the agent is local (e.g., town-level agents).
func LocalBackend() Backend {
	return NewTmuxBackend(tmux.NewTmux())
}

// resolveCoopConfig checks agent bead metadata for Coop sidecar configuration.
// Returns nil if the agent doesn't use Coop.
func resolveCoopConfig(agentID string) (*coopResolvedConfig, error) {
	notes, err := getAgentNotes(agentID)
	if err != nil {
		return nil, fmt.Errorf("agent bead lookup failed: %w", err)
	}
	return parseCoopConfig(notes)
}

// coopResolvedConfig holds Coop connection info parsed from bead metadata.
type coopResolvedConfig struct {
	CoopConfig
	baseURL string
}

// parseCoopConfig parses Coop config from bd show output.
// Returns nil if the output doesn't indicate a Coop agent.
func parseCoopConfig(output string) (*coopResolvedConfig, error) {
	outStr := strings.TrimSpace(output)
	if outStr == "" || !strings.Contains(outStr, "coop") {
		return nil, nil // Not a Coop agent
	}

	cfg := &coopResolvedConfig{}
	for _, line := range strings.Split(outStr, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "coop_url":
			cfg.baseURL = val
		case "coop_token":
			cfg.Token = val
		}
	}

	if cfg.baseURL == "" {
		return nil, fmt.Errorf("Coop agent missing coop_url in bead metadata")
	}

	return cfg, nil
}

// resolveSSHConfig checks agent bead metadata to determine if this agent
// is K8s-hosted and returns SSH connection config if so.
//
// Returns nil if the agent is local or metadata is unavailable.
func resolveSSHConfig(agentID string) (*SSHConfig, error) {
	notes, err := getAgentNotes(agentID)
	if err != nil {
		return nil, fmt.Errorf("agent bead lookup failed: %w", err)
	}
	return parseSSHConfig(notes)
}

// parseSSHConfig parses SSH connection config from bd show output.
// Returns nil if the output doesn't indicate a K8s agent.
func parseSSHConfig(output string) (*SSHConfig, error) {
	outStr := strings.TrimSpace(output)
	if outStr == "" || !strings.Contains(outStr, "k8s") {
		return nil, nil // Not a K8s agent
	}

	// Parse SSH config from bead metadata
	// This will be populated by Phase 4's pod manager when creating K8s polecats
	cfg := &SSHConfig{Port: 22}

	for _, line := range strings.Split(outStr, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "ssh_host":
			cfg.Host = val
		case "ssh_port":
			cfg.Port = parsePort(val)
		case "ssh_key":
			cfg.IdentityFile = val
		}
	}

	if cfg.Host == "" {
		return nil, fmt.Errorf("K8s agent missing ssh_host in bead metadata")
	}

	return cfg, nil
}

// getAgentNotes fetches the notes field from an agent bead via bd show --json.
// Backend metadata (backend, coop_url, ssh_host, etc.) is stored in the notes
// field as key: value pairs, one per line.
func getAgentNotes(agentID string) (string, error) {
	cmd := exec.Command("bd", "show", agentID, "--json") //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bd show failed: %w", err)
	}

	// bd show --json returns an array of issues
	var issues []struct {
		Notes string `json:"notes"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		return "", fmt.Errorf("failed to parse bd show output: %w", err)
	}
	if len(issues) == 0 {
		return "", fmt.Errorf("agent bead %q not found", agentID)
	}
	return issues[0].Notes, nil
}

// parsePort parses a port string to int, defaulting to 22.
func parsePort(s string) int {
	var port int
	if _, err := fmt.Sscanf(s, "%d", &port); err != nil || port <= 0 {
		return 22
	}
	return port
}
