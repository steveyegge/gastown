// Package cmd provides CLI commands for the gt tool.
// This file implements the gt config seed command for creating config beads
// from existing embedded template files.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	seedDryRun   bool
	seedHooks    bool
	seedMCP      bool
	seedAccounts bool
	seedDaemon   bool
	seedForce    bool
)

var configSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed config beads from embedded templates",
	Long: `Create config beads from the embedded Claude settings and MCP templates.

This is a one-time migration command that reads the existing embedded config
templates (settings-autonomous.json, settings-interactive.json, mcp.json)
and creates corresponding config beads in the beads database.

By default, seeds all config types. Use flags to seed specific types:

  gt config seed              # Seed all config types
  gt config seed --hooks      # Only Claude hooks
  gt config seed --mcp        # Only MCP config
  gt config seed --accounts   # Only account config
  gt config seed --daemon     # Only daemon patrol config
  gt config seed --dry-run    # Show what would be created
  gt config seed --force      # Overwrite existing beads`,
	RunE: runConfigSeed,
}

func init() {
	configSeedCmd.Flags().BoolVar(&seedDryRun, "dry-run", false, "Show what would be created without creating")
	configSeedCmd.Flags().BoolVar(&seedHooks, "hooks", false, "Only seed Claude hook config beads")
	configSeedCmd.Flags().BoolVar(&seedMCP, "mcp", false, "Only seed MCP config beads")
	configSeedCmd.Flags().BoolVar(&seedAccounts, "accounts", false, "Only seed account config beads")
	configSeedCmd.Flags().BoolVar(&seedDaemon, "daemon", false, "Only seed daemon patrol config beads")
	configSeedCmd.Flags().BoolVar(&seedForce, "force", false, "Overwrite existing config beads")

	configCmd.AddCommand(configSeedCmd)
}

func runConfigSeed(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	bd := beads.New(townRoot)

	// If no specific flags, seed everything
	seedAll := !seedHooks && !seedMCP && !seedAccounts && !seedDaemon

	var created, skipped, updated int

	if seedAll || seedHooks {
		c, s, u, err := seedHookBeads(bd)
		if err != nil {
			return fmt.Errorf("seeding hook beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedMCP {
		c, s, u, err := seedMCPBeads(bd)
		if err != nil {
			return fmt.Errorf("seeding MCP beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedAccounts {
		c, s, u, err := seedAccountBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding account beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedDaemon {
		c, s, u, err := seedDaemonBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding daemon beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	// Summary
	fmt.Println()
	if seedDryRun {
		fmt.Printf("%s Dry run complete: would create %d, would skip %d, would update %d\n",
			style.Info.Render("ℹ"), created, skipped, updated)
	} else {
		fmt.Printf("%s Seed complete: created %d, skipped %d, updated %d\n",
			style.Success.Render("✓"), created, skipped, updated)
	}

	return nil
}

// seedHookBeads creates config beads for Claude hook settings.
// It reads the embedded templates, diffs them to find shared vs role-specific
// settings, and creates:
//   - hq-cfg-hooks-base: shared settings (PreToolUse, PreCompact, PostToolUse, UserPromptSubmit)
//   - hq-cfg-hooks-polecat: polecat-specific overrides (SessionStart with mail check, Stop with --soft)
//   - hq-cfg-hooks-crew: crew-specific overrides (SessionStart without mail check, Stop without --soft)
func seedHookBeads(bd *beads.Beads) (created, skipped, updated int, err error) {
	// Read embedded templates
	autoContent, err := claude.TemplateContent(claude.Autonomous)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reading autonomous template: %w", err)
	}
	interContent, err := claude.TemplateContent(claude.Interactive)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reading interactive template: %w", err)
	}

	// Parse both templates
	var autoSettings map[string]interface{}
	var interSettings map[string]interface{}
	if err := json.Unmarshal(autoContent, &autoSettings); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing autonomous template: %w", err)
	}
	if err := json.Unmarshal(interContent, &interSettings); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing interactive template: %w", err)
	}

	// Extract hooks from both
	autoHooks := extractHooksMap(autoSettings)
	interHooks := extractHooksMap(interSettings)

	// Identify shared vs different hooks
	baseHooks := make(map[string]interface{})
	autoOnlyHooks := make(map[string]interface{})
	interOnlyHooks := make(map[string]interface{})

	// All hook names from both templates
	allHookNames := make(map[string]bool)
	for k := range autoHooks {
		allHookNames[k] = true
	}
	for k := range interHooks {
		allHookNames[k] = true
	}

	for name := range allHookNames {
		autoJSON, _ := json.Marshal(autoHooks[name])
		interJSON, _ := json.Marshal(interHooks[name])

		if string(autoJSON) == string(interJSON) {
			// Shared between both roles
			baseHooks[name] = autoHooks[name]
		} else {
			// Different between roles
			if autoHooks[name] != nil {
				autoOnlyHooks[name] = autoHooks[name]
			}
			if interHooks[name] != nil {
				interOnlyHooks[name] = interHooks[name]
			}
		}
	}

	// Build base settings (non-hook fields + shared hooks)
	baseSettings := make(map[string]interface{})
	for k, v := range autoSettings {
		if k != "hooks" {
			baseSettings[k] = v
		}
	}
	if len(baseHooks) > 0 {
		baseSettings["hooks"] = baseHooks
	}

	// Create base config bead
	c, s, u, err := createOrSkipConfigBead(bd, "hooks-base", beads.ConfigCategoryClaudeHooks,
		"*", "", "", baseSettings, "Base Claude hooks shared by all roles")
	if err != nil {
		return 0, 0, 0, err
	}
	created += c
	skipped += s
	updated += u

	// Create polecat-specific bead (autonomous roles)
	if len(autoOnlyHooks) > 0 {
		polecatOverride := map[string]interface{}{
			"hooks": autoOnlyHooks,
		}
		c, s, u, err = createOrSkipConfigBead(bd, "hooks-polecat", beads.ConfigCategoryClaudeHooks,
			"*", "polecat", "", polecatOverride, "Polecat-specific hook overrides")
		if err != nil {
			return created, skipped, updated, err
		}
		created += c
		skipped += s
		updated += u
	}

	// Create crew-specific bead (interactive roles)
	if len(interOnlyHooks) > 0 {
		crewOverride := map[string]interface{}{
			"hooks": interOnlyHooks,
		}
		c, s, u, err = createOrSkipConfigBead(bd, "hooks-crew", beads.ConfigCategoryClaudeHooks,
			"*", "crew", "", crewOverride, "Crew-specific hook overrides")
		if err != nil {
			return created, skipped, updated, err
		}
		created += c
		skipped += s
		updated += u
	}

	return created, skipped, updated, nil
}

// seedMCPBeads creates config beads for MCP server configuration.
func seedMCPBeads(bd *beads.Beads) (created, skipped, updated int, err error) {
	// Read embedded MCP template using the claude package's embed FS
	mcpContent, err := claude.MCPTemplateContent()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reading MCP template: %w", err)
	}

	var mcpConfig map[string]interface{}
	if err := json.Unmarshal(mcpContent, &mcpConfig); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing MCP template: %w", err)
	}

	return createOrSkipConfigBead(bd, "mcp-global", beads.ConfigCategoryMCP,
		"*", "", "", mcpConfig, "Global MCP server configuration")
}

// createOrSkipConfigBead creates a config bead or skips if it already exists.
// Returns (created, skipped, updated) counts.
func createOrSkipConfigBead(bd *beads.Beads, slug, category, rig, role, agent string,
	metadata interface{}, description string) (created, skipped, updated int, err error) {

	id := beads.ConfigBeadID(slug)

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshaling metadata for %s: %w", slug, err)
	}

	// Check if bead already exists
	existing, _, getErr := bd.GetConfigBead(id)
	if getErr != nil {
		return 0, 0, 0, fmt.Errorf("checking existing bead %s: %w", id, getErr)
	}

	action := "Created"
	if existing != nil {
		if seedForce {
			// Update existing bead
			action = "Updated"
			if seedDryRun {
				fmt.Printf("  %s Would update %s (%s)\n", style.Warning.Render("~"), id, description)
				return 0, 0, 1, nil
			}
			err = bd.UpdateConfigMetadata(id, string(metadataJSON))
			if err != nil {
				return 0, 0, 0, fmt.Errorf("updating %s: %w", id, err)
			}
			fmt.Printf("  %s %s %s (%s)\n", style.Success.Render("✓"), action, id, description)
			return 0, 0, 1, nil
		}
		// Skip existing
		fmt.Printf("  - Skipped %s (already exists)\n", id)
		return 0, 1, 0, nil
	}

	if seedDryRun {
		fmt.Printf("  %s Would create %s (%s)\n", style.Info.Render("+"), id, description)
		return 1, 0, 0, nil
	}

	// Create the bead
	fields := &beads.ConfigFields{
		Rig:      rig,
		Category: category,
		Metadata: string(metadataJSON),
	}

	_, err = bd.CreateConfigBead(slug, fields, role, agent)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("creating %s: %w", id, err)
	}

	fmt.Printf("  %s %s %s (%s)\n", style.Success.Render("✓"), action, id, description)
	return 1, 0, 0, nil
}

// seedAccountBeads creates config beads for account configuration.
// It reads mayor/accounts.json and creates:
//   - hq-cfg-account-<handle>: per-account config (excluding auth_token)
//   - hq-cfg-accounts-default: default account selection
func seedAccountBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := config.LoadAccountsConfig(accountsPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loading accounts config: %w", err)
	}

	// Create a bead for each account (excluding auth_token - it's a SECRET)
	for handle, acct := range cfg.Accounts {
		metadata := map[string]interface{}{
			"handle":     handle,
			"config_dir": acct.ConfigDir,
		}
		if acct.Email != "" {
			metadata["email"] = acct.Email
		}
		if acct.Description != "" {
			metadata["description"] = acct.Description
		}
		if acct.BaseURL != "" {
			metadata["base_url"] = acct.BaseURL
		}
		// NOTE: auth_token is intentionally excluded - secrets never go in beads

		slug := "account-" + handle
		desc := fmt.Sprintf("Account config: %s", handle)
		c, s, u, err := createOrSkipConfigBead(bd, slug, beads.ConfigCategoryAccounts,
			"*", "", "", metadata, desc)
		if err != nil {
			return created, skipped, updated, fmt.Errorf("creating account bead for %s: %w", handle, err)
		}
		created += c
		skipped += s
		updated += u
	}

	// Create the default-account bead
	if cfg.Default != "" {
		defaultMeta := map[string]interface{}{
			"default": cfg.Default,
		}
		c, s, u, err := createOrSkipConfigBead(bd, "accounts-default", beads.ConfigCategoryAccounts,
			"*", "", "", defaultMeta, "Default account selection")
		if err != nil {
			return created, skipped, updated, fmt.Errorf("creating default account bead: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	return created, skipped, updated, nil
}

// seedDaemonBeads creates a config bead for daemon patrol configuration.
// It reads the existing mayor/daemon.json file (or uses defaults if not found)
// and creates a single global config bead: hq-cfg-daemon-patrol.
func seedDaemonBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	// Read daemon.json from filesystem
	configPath := config.DaemonPatrolConfigPath(townRoot)
	daemonConfig, loadErr := config.LoadDaemonPatrolConfig(configPath)
	if loadErr != nil {
		if errors.Is(loadErr, config.ErrNotFound) {
			// No daemon.json exists, use defaults
			daemonConfig = config.NewDaemonPatrolConfig()
		} else {
			return 0, 0, 0, fmt.Errorf("reading daemon patrol config: %w", loadErr)
		}
	}

	// Marshal to generic map for bead metadata storage
	daemonJSON, err := json.Marshal(daemonConfig)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshaling daemon patrol config: %w", err)
	}

	var daemonMap map[string]interface{}
	if err := json.Unmarshal(daemonJSON, &daemonMap); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing daemon patrol config: %w", err)
	}

	return createOrSkipConfigBead(bd, "daemon-patrol", beads.ConfigCategoryDaemon,
		"*", "", "", daemonMap, "Global daemon patrol configuration")
}

// extractHooksMap extracts the "hooks" key from a settings map.
func extractHooksMap(settings map[string]interface{}) map[string]interface{} {
	hooks, ok := settings["hooks"]
	if !ok {
		return nil
	}
	hooksMap, ok := hooks.(map[string]interface{})
	if !ok {
		return nil
	}
	return hooksMap
}

