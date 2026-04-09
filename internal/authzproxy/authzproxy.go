// Package authzproxy generates authz files and .mcp.json for polecat worktrees.
//
// When polecats are dispatched with --mcp or --gcp flags, this package:
//  1. Generates an mcp-authz.json file defining the polecat's MCP/GCP permissions
//  2. Generates a .mcp.json file in the worktree root pointing each MCP at the
//     authz-proxy frontend binary
//  3. Returns the MCP tool permission patterns for .claude/settings.json injection
//
// The authz-proxy daemon (separate binary) runs on a Unix socket and handles
// upstream MCP server management + GCP SA impersonation. The frontend binary
// is a thin stdio<->socket bridge that Claude Code spawns as an MCP server.
package authzproxy

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// MCPPolicy defines access for a single MCP server in the authz context.
type MCPPolicy struct {
	Mode  string   `json:"mode"`            // "read" or "read,write"
	Tools []string `json:"tools,omitempty"` // glob patterns, empty = all
}

// GCPProfile defines a GCP SA impersonation target.
type GCPProfile struct {
	TargetSA string   `json:"target_sa"`
	Scopes   []string `json:"scopes"`
	Lifetime string   `json:"lifetime,omitempty"`
}

// GCPAuthz defines GCP credential access for a client.
type GCPAuthz struct {
	Profiles map[string]GCPProfile `json:"profiles"`
}

// AuthzContext is the authorization context written to mcp-authz.json.
type AuthzContext struct {
	Role    string               `json:"role"`
	AgentID string               `json:"agent_id"`
	Bead    string               `json:"bead"`
	MCPs    map[string]MCPPolicy `json:"mcps"`
	GCP     *GCPAuthz            `json:"gcp,omitempty"`
}

// MCPServerEntry is an entry in .mcp.json's mcpServers map.
type MCPServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// MCPConfig is the structure written to .mcp.json.
type MCPConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// Config holds the authz-proxy paths needed for generation.
type Config struct {
	Binary      string // Path to the authz-proxy binary
	Socket      string // Path to the daemon's Unix socket
	SecretsPath string // Path to .mcp-secrets.json (for GCP profile resolution)
}

// ParseMCPPolicy parses a --mcp flag value like "github:read" or "linear:read,write".
// If no mode is specified, defaults to "read".
func ParseMCPPolicy(spec string) (name string, policy MCPPolicy, err error) {
	if spec == "" {
		return "", MCPPolicy{}, fmt.Errorf("empty MCP policy spec")
	}
	parts := strings.SplitN(spec, ":", 2)
	name = parts[0]
	if name == "" {
		return "", MCPPolicy{}, fmt.Errorf("empty MCP name in spec %q", spec)
	}
	mode := "read"
	if len(parts) == 2 && parts[1] != "" {
		mode = parts[1]
	}
	// Validate mode
	switch mode {
	case "read", "read,write", "write":
		// valid
	default:
		return "", MCPPolicy{}, fmt.Errorf("invalid MCP mode %q in spec %q: must be read, write, or read,write", mode, spec)
	}
	return name, MCPPolicy{Mode: mode, Tools: []string{"*"}}, nil
}

// SecretsFile represents the structure of .mcp-secrets.json (partially).
type SecretsFile struct {
	GCPProfiles map[string]GCPProfile `json:"gcp_profiles"`
}

// ResolveGCPProfiles reads .mcp-secrets.json and returns the requested GCP profiles.
func ResolveGCPProfiles(secretsPath string, profileNames []string) (map[string]GCPProfile, error) {
	if len(profileNames) == 0 {
		return nil, nil
	}
	if secretsPath == "" {
		return nil, fmt.Errorf("--gcp requires authz_proxy.secrets_path in town settings")
	}
	data, err := os.ReadFile(secretsPath) //nolint:gosec // G304: path from config
	if err != nil {
		return nil, fmt.Errorf("reading secrets file: %w", err)
	}
	var secrets SecretsFile
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("parsing secrets file: %w", err)
	}
	profiles := make(map[string]GCPProfile, len(profileNames))
	for _, name := range profileNames {
		p, ok := secrets.GCPProfiles[name]
		if !ok {
			return nil, fmt.Errorf("GCP profile %q not found in secrets file %s", name, secretsPath)
		}
		profiles[name] = p
	}
	return profiles, nil
}

// GenerateAuthzFile creates the mcp-authz.json file in the given directory.
// Returns the path to the generated file.
func GenerateAuthzFile(dir string, ctx AuthzContext) (string, error) {
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling authz context: %w", err)
	}
	authzPath := filepath.Join(dir, "mcp-authz.json")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating authz directory: %w", err)
	}
	if err := os.WriteFile(authzPath, append(data, '\n'), 0644); err != nil {
		return "", fmt.Errorf("writing authz file: %w", err)
	}
	return authzPath, nil
}

// GenerateMCPConfig creates the .mcp.json file in the given worktree root.
// Each MCP in the authz context gets an entry pointing to the frontend binary.
func GenerateMCPConfig(worktreeRoot string, authzPath string, cfg Config) (string, error) {
	mcpCfg := MCPConfig{
		MCPServers: make(map[string]MCPServerEntry),
	}
	// Read the authz file to enumerate MCPs
	data, err := os.ReadFile(authzPath) //nolint:gosec // G304: path we just wrote
	if err != nil {
		return "", fmt.Errorf("reading authz file: %w", err)
	}
	var ctx AuthzContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return "", fmt.Errorf("parsing authz file: %w", err)
	}

	for mcpName := range ctx.MCPs {
		mcpCfg.MCPServers[mcpName] = MCPServerEntry{
			Command: cfg.Binary,
			Args:    []string{"frontend", "--authz", authzPath, "--socket", cfg.Socket},
		}
	}

	mcpData, err := json.MarshalIndent(mcpCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling .mcp.json: %w", err)
	}
	mcpPath := filepath.Join(worktreeRoot, ".mcp.json")
	if err := os.WriteFile(mcpPath, append(mcpData, '\n'), 0644); err != nil {
		return "", fmt.Errorf("writing .mcp.json: %w", err)
	}
	return mcpPath, nil
}

// MCPToolPermissions returns the Claude Code tool permission patterns for the given MCPs.
// Each MCP gets a pattern like "mcp__github__*" that should be added to
// .claude/settings.json permissions.allow.
func MCPToolPermissions(mcpNames []string) []string {
	perms := make([]string, 0, len(mcpNames))
	for _, name := range mcpNames {
		perms = append(perms, fmt.Sprintf("mcp__%s__*", name))
	}
	return perms
}

// CheckDaemonSocket verifies the authz-proxy daemon is running by checking
// that the Unix socket exists and is connectable.
func CheckDaemonSocket(socketPath string) error {
	info, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("authz-proxy daemon socket not found at %s (is the daemon running?)", socketPath)
		}
		return fmt.Errorf("checking socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists but is not a Unix socket", socketPath)
	}
	// Try a quick connect to verify the daemon is responsive
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("authz-proxy daemon socket exists but is not connectable at %s: %w", socketPath, err)
	}
	conn.Close()
	return nil
}
