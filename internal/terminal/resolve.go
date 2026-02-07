package terminal

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/tmux"
)

// ResolveBackend returns the appropriate Backend for the given agent.
// For local agents, returns a TmuxBackend. For K8s-hosted agents,
// returns an SSHBackend configured to reach the pod.
//
// The agentID follows the standard format: "rig/polecat" or "rig/crew/name".
// Backend detection checks the agent bead for a "backend" field set by
// Phase 4's K8s pod manager.
func ResolveBackend(agentID string) Backend {
	// Check if agent has K8s backend metadata
	cfg, err := resolveSSHConfig(agentID)
	if err == nil && cfg != nil {
		return NewSSHBackend(*cfg)
	}

	// Default: local tmux
	return NewTmuxBackend(tmux.NewTmux())
}

// LocalBackend returns a TmuxBackend for local tmux operations.
// Use this when you know the agent is local (e.g., town-level agents).
func LocalBackend() Backend {
	return NewTmuxBackend(tmux.NewTmux())
}

// resolveSSHConfig checks agent bead metadata to determine if this agent
// is K8s-hosted and returns SSH connection config if so.
//
// Returns nil if the agent is local or metadata is unavailable.
func resolveSSHConfig(agentID string) (*SSHConfig, error) {
	// Query agent bead for backend metadata.
	// Phase 4 will set fields like:
	//   backend: k8s
	//   pod_name: gt-gastown-toast-xxxxx
	//   pod_namespace: gastown
	//   ssh_host: gt@gt-gastown-toast-xxxxx.gastown.svc.cluster.local
	//   ssh_port: 22
	//
	// For now, we also support environment variable override for testing:
	//   GT_SSH_HOST=gt@localhost GT_SSH_PORT=2222 gt peek gastown/toast

	// Try bd show --json to get agent bead metadata
	cmd := exec.Command("bd", "show", agentID, "--json", "--field=backend,ssh_host,ssh_port,ssh_key") //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agent bead lookup failed: %w", err)
	}

	return parseSSHConfig(string(output))
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

// parsePort parses a port string to int, defaulting to 22.
func parsePort(s string) int {
	var port int
	if _, err := fmt.Sscanf(s, "%d", &port); err != nil || port <= 0 {
		return 22
	}
	return port
}
