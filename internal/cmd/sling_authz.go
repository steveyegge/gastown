package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/authzproxy"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
)

// injectAuthzProxy generates mcp-authz.json and .mcp.json in the polecat's worktree,
// and adds MCP tool permissions to the polecat's .claude/settings.json.
// This gives the polecat scoped access to MCP servers and GCP credentials via the
// authz-proxy daemon.
func injectAuthzProxy(townRoot, worktreeRoot, agentID, beadID string, mcpSpecs, gcpProfiles []string) error {
	// Load authz-proxy config from town settings
	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	proxyCfg := settings.AuthzProxy
	if proxyCfg == nil {
		return fmt.Errorf("authz_proxy not configured in town settings (settings/config.json)")
	}
	if proxyCfg.Binary == "" {
		return fmt.Errorf("authz_proxy.binary not set in town settings")
	}
	if proxyCfg.Socket == "" {
		return fmt.Errorf("authz_proxy.socket not set in town settings")
	}

	// Verify the daemon socket is reachable
	if err := authzproxy.CheckDaemonSocket(proxyCfg.Socket); err != nil {
		return err
	}

	// Parse MCP policy specs
	mcps := make(map[string]authzproxy.MCPPolicy)
	var mcpNames []string
	for _, spec := range mcpSpecs {
		name, policy, err := authzproxy.ParseMCPPolicy(spec)
		if err != nil {
			return fmt.Errorf("parsing --mcp %q: %w", spec, err)
		}
		mcps[name] = policy
		mcpNames = append(mcpNames, name)
	}

	// Resolve GCP profiles from secrets file
	var gcpAuthz *authzproxy.GCPAuthz
	if len(gcpProfiles) > 0 {
		profiles, err := authzproxy.ResolveGCPProfiles(proxyCfg.SecretsPath, gcpProfiles)
		if err != nil {
			return fmt.Errorf("resolving GCP profiles: %w", err)
		}
		gcpAuthz = &authzproxy.GCPAuthz{Profiles: profiles}
	}

	// Generate the authz file in .bridge/ within the worktree
	bridgeDir := filepath.Join(worktreeRoot, ".bridge")
	authzCtx := authzproxy.AuthzContext{
		Role:    "polecat",
		AgentID: agentID,
		Bead:    beadID,
		MCPs:    mcps,
		GCP:     gcpAuthz,
	}
	authzPath, err := authzproxy.GenerateAuthzFile(bridgeDir, authzCtx)
	if err != nil {
		return fmt.Errorf("generating authz file: %w", err)
	}
	fmt.Printf("  %s Authz file: %s\n", style.Bold.Render("✓"), authzPath)

	// Generate .mcp.json in the worktree root
	cfg := authzproxy.Config{
		Binary:      proxyCfg.Binary,
		Socket:      proxyCfg.Socket,
		SecretsPath: proxyCfg.SecretsPath,
	}
	mcpPath, err := authzproxy.GenerateMCPConfig(worktreeRoot, authzPath, cfg)
	if err != nil {
		return fmt.Errorf("generating .mcp.json: %w", err)
	}
	fmt.Printf("  %s MCP config: %s\n", style.Bold.Render("✓"), mcpPath)

	// Add MCP tool permissions to .claude/settings.json
	if len(mcpNames) > 0 {
		if err := addMCPPermissionsToSettings(worktreeRoot, mcpNames); err != nil {
			// Warn but continue — the polecat settings may be at a parent directory
			fmt.Printf("  %s Could not update .claude/settings.json: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("  %s MCP permissions added to settings\n", style.Bold.Render("✓"))
		}
	}

	return nil
}

// addMCPPermissionsToSettings adds MCP tool permission patterns and enables
// project MCP servers in the polecat's .claude/settings.json.
func addMCPPermissionsToSettings(worktreeRoot string, mcpNames []string) error {
	// The polecat settings may be at the worktree's .claude/settings.json,
	// or at a parent directory (polecats/.claude/settings.json).
	// Try the worktree first, then check parent directories.
	settingsPath := filepath.Join(worktreeRoot, ".claude", "settings.json")

	// If no settings.json at the worktree level, check the polecats parent dir
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		// Try polecats/.claude/settings.json (shared polecat settings)
		parentDir := filepath.Dir(worktreeRoot)
		parentSettingsPath := filepath.Join(parentDir, ".claude", "settings.json")
		if _, err := os.Stat(parentSettingsPath); err == nil {
			settingsPath = parentSettingsPath
		} else {
			// Create settings.json at the worktree level
			if err := os.MkdirAll(filepath.Join(worktreeRoot, ".claude"), 0755); err != nil {
				return fmt.Errorf("creating .claude dir: %w", err)
			}
			initial := map[string]interface{}{
				"permissions": map[string]interface{}{
					"allow": []string{},
				},
			}
			data, _ := json.MarshalIndent(initial, "", "  ")
			if err := os.WriteFile(settingsPath, data, 0644); err != nil {
				return fmt.Errorf("creating settings.json: %w", err)
			}
		}
	}

	// Read existing settings
	data, err := os.ReadFile(settingsPath) //nolint:gosec // G304: path constructed internally
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	// Get or create permissions.allow
	perms, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		perms = map[string]interface{}{"allow": []interface{}{}}
		settings["permissions"] = perms
	}

	allowRaw, ok := perms["allow"]
	var allow []interface{}
	if ok {
		allow, _ = allowRaw.([]interface{})
	}

	// Add MCP tool permissions (dedup against existing)
	existingPerms := make(map[string]bool)
	for _, a := range allow {
		if s, ok := a.(string); ok {
			existingPerms[s] = true
		}
	}
	for _, perm := range authzproxy.MCPToolPermissions(mcpNames) {
		if !existingPerms[perm] {
			allow = append(allow, perm)
		}
	}
	perms["allow"] = allow

	// Enable all project MCP servers
	settings["enableAllProjectMcpServers"] = true

	// Write back
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	return nil
}
