package terminal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/bdcmd"
)

// ResolveBackend returns the appropriate Backend for the given agent.
// All agents are Coop-backed in the K8s-only architecture.
//
// The agentID follows the standard format: "rig/polecat" or "rig/crew/name".
// Backend detection checks the agent bead for a "backend" field set by
// the K8s pod manager or Coop sidecar deployment.
func ResolveBackend(agentID string) Backend {
	// Try the given agentID first, then hq-prefixed form for town-level
	// shortnames (mayor -> hq-mayor, deacon -> hq-deacon, etc.).
	candidates := []string{agentID}
	if !strings.Contains(agentID, "/") && !strings.Contains(agentID, "-") {
		candidates = append(candidates, "hq-"+agentID)
	}

	for _, id := range candidates {
		// Check if agent has Coop backend metadata
		coopCfg, err := resolveCoopConfig(id)
		if err == nil && coopCfg != nil {
			b := NewCoopBackend(coopCfg.CoopConfig)
			b.AddSession("claude", coopCfg.baseURL)
			return b
		}
	}

	// Default: return a Coop backend with no sessions configured.
	// Callers should check for errors when invoking methods.
	return NewCoopBackend(CoopConfig{})
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
	baseURL   string
	podName   string
	namespace string
}

// AgentPodInfo contains K8s pod metadata for an agent.
type AgentPodInfo struct {
	PodName   string
	Namespace string
	CoopURL   string
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
		case "pod_name":
			cfg.podName = val
		case "pod_namespace":
			cfg.namespace = val
		}
	}

	if cfg.baseURL == "" {
		return nil, fmt.Errorf("Coop agent missing coop_url in bead metadata")
	}

	return cfg, nil
}

// getAgentNotes fetches the notes field from an agent bead via bd show --json.
// Backend metadata (backend, coop_url, etc.) is stored in the notes
// field as key: value pairs, one per line.
func getAgentNotes(agentID string) (string, error) {
	cmd := bdcmd.Command("show", agentID, "--json")
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

// ResolveAgentPodInfo looks up an agent's K8s pod metadata from its bead notes.
// The address can be a shortname ("mayor"), bead ID ("hq-mayor"), or path
// ("gastown/polecats/furiosa"). Returns pod_name and pod_namespace from the
// bead's notes field, which are written by the controller's status reporter.
func ResolveAgentPodInfo(address string) (*AgentPodInfo, error) {
	// Build candidate bead IDs to try.
	candidates := []string{address}

	// Try parsing as an address to get bead ID format.
	// Import cycle avoidance: we parse the address format inline.
	switch address {
	case "mayor":
		candidates = []string{"hq-mayor"}
	case "deacon":
		candidates = []string{"hq-deacon"}
	case "boot":
		candidates = []string{"hq-boot"}
	default:
		// If it contains slashes, it's a path format — add the hyphenated form.
		if strings.Contains(address, "/") {
			parts := strings.Split(address, "/")
			switch len(parts) {
			case 2:
				// rig/role → gt-rig-role-hq
				candidates = append(candidates, fmt.Sprintf("gt-%s-%s-hq", parts[0], parts[1]))
			case 3:
				// rig/type/name → gt-rig-type-name
				role := parts[1]
				if role == "polecats" {
					role = "polecat"
				}
				candidates = append(candidates, fmt.Sprintf("gt-%s-%s-%s", parts[0], role, parts[2]))
			}
		}
	}

	for _, id := range candidates {
		cfg, err := resolveCoopConfig(id)
		if err != nil || cfg == nil {
			continue
		}
		if cfg.podName != "" {
			return &AgentPodInfo{
				PodName:   cfg.podName,
				Namespace: cfg.namespace,
				CoopURL:   cfg.baseURL,
			}, nil
		}
	}

	return nil, fmt.Errorf("no pod metadata found for agent %q", address)
}
