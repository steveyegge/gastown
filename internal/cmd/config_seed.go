// Package cmd provides CLI commands for the gt tool.
// This file implements the gt config seed command for creating config beads
// from existing embedded template files.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	seedDryRun     bool
	seedHooks      bool
	seedMCP        bool
	seedIdentity   bool
	seedAccounts   bool
	seedDaemon     bool
	seedRigs       bool
	seedAgents     bool
	seedSlack      bool
	seedMessaging  bool
	seedEscalation bool
	seedForce      bool
)

var configSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed config beads from embedded templates",
	Long: `Create config beads from existing configuration files and templates.

This is a one-time migration command that reads existing config files and
embedded templates and creates corresponding config beads in the beads database.

By default, seeds all config types. Use flags to seed specific types:

  gt config seed              # Seed all config types
  gt config seed --hooks      # Only Claude hooks
  gt config seed --mcp        # Only MCP config
  gt config seed --identity   # Only town identity
  gt config seed --accounts   # Only account config
  gt config seed --daemon     # Only daemon patrol config
  gt config seed --rigs       # Only rig registry
  gt config seed --agents     # Only agent preset config
  gt config seed --slack      # Only Slack routing config
  gt config seed --messaging  # Only messaging config
  gt config seed --escalation # Only escalation config
  gt config seed --dry-run    # Show what would be created
  gt config seed --force      # Overwrite existing beads`,
	RunE: runConfigSeed,
}

func init() {
	configSeedCmd.Flags().BoolVar(&seedDryRun, "dry-run", false, "Show what would be created without creating")
	configSeedCmd.Flags().BoolVar(&seedHooks, "hooks", false, "Only seed Claude hook config beads")
	configSeedCmd.Flags().BoolVar(&seedMCP, "mcp", false, "Only seed MCP config beads")
	configSeedCmd.Flags().BoolVar(&seedIdentity, "identity", false, "Only seed town identity config beads")
	configSeedCmd.Flags().BoolVar(&seedAccounts, "accounts", false, "Only seed account config beads")
	configSeedCmd.Flags().BoolVar(&seedDaemon, "daemon", false, "Only seed daemon patrol config beads")
	configSeedCmd.Flags().BoolVar(&seedRigs, "rigs", false, "Only seed rig registry config beads")
	configSeedCmd.Flags().BoolVar(&seedAgents, "agents", false, "Only seed agent preset config beads")
	configSeedCmd.Flags().BoolVar(&seedSlack, "slack", false, "Only seed Slack routing config beads")
	configSeedCmd.Flags().BoolVar(&seedMessaging, "messaging", false, "Only seed messaging config beads")
	configSeedCmd.Flags().BoolVar(&seedEscalation, "escalation", false, "Only seed escalation config beads")
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
	seedAll := !seedHooks && !seedMCP && !seedIdentity && !seedAccounts && !seedDaemon &&
		!seedRigs && !seedAgents && !seedSlack && !seedMessaging && !seedEscalation

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

	if seedAll || seedIdentity {
		c, s, u, err := seedIdentityBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding identity beads: %w", err)
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

	if seedAll || seedRigs {
		c, s, u, err := seedRigRegistryBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding rig registry beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedAgents {
		c, s, u, err := seedAgentPresetBeads(bd)
		if err != nil {
			return fmt.Errorf("seeding agent preset beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedSlack {
		c, s, u, err := seedSlackBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding slack beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedMessaging {
		c, s, u, err := seedMessagingBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding messaging beads: %w", err)
		}
		created += c
		skipped += s
		updated += u
	}

	if seedAll || seedEscalation {
		c, s, u, err := seedEscalationBeads(bd, townRoot)
		if err != nil {
			return fmt.Errorf("seeding escalation beads: %w", err)
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

// seedIdentityBeads creates a config bead for town identity from mayor/town.json.
func seedIdentityBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	townConfigPath := filepath.Join(townRoot, workspace.PrimaryMarker)
	tc, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loading town config: %w", err)
	}

	// Build metadata from TownConfig fields
	metadata := map[string]interface{}{
		"type":       tc.Type,
		"version":    tc.Version,
		"name":       tc.Name,
		"created_at": tc.CreatedAt,
	}
	if tc.Owner != "" {
		metadata["owner"] = tc.Owner
	}
	if tc.PublicName != "" {
		metadata["public_name"] = tc.PublicName
	}

	slug := "town-" + tc.Name
	return createOrSkipConfigBead(bd, slug, beads.ConfigCategoryIdentity,
		tc.Name, "", "", metadata, "Town identity: "+tc.Name)
}

// seedRigRegistryBeads creates config beads for each rig in the registry.
// For each rig, it creates:
//   - hq-cfg-rig-<town>-<rigName>: from rigs.json entry (registry metadata)
//   - hq-cfg-rigcfg-<town>-<rigName>: from rig/config.json (rig identity)
func seedRigRegistryBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	// Load town config to get town name
	townCfg, err := config.LoadTownConfig(filepath.Join(townRoot, "mayor", "town.json"))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loading town config: %w", err)
	}
	townName := townCfg.Name

	// Load rigs registry
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsCfg, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loading rigs config: %w", err)
	}

	for rigName, entry := range rigsCfg.Rigs {
		rigScope := townName + "/" + rigName

		// 1. Create rig registry bead from rigs.json entry
		slug := "rig-" + townName + "-" + rigName
		metadata := map[string]interface{}{
			"git_url":    entry.GitURL,
			"local_repo": entry.LocalRepo,
			"added_at":   entry.AddedAt,
		}
		if entry.BeadsConfig != nil {
			metadata["beads"] = map[string]interface{}{
				"repo":   entry.BeadsConfig.Repo,
				"prefix": entry.BeadsConfig.Prefix,
			}
		}

		desc := fmt.Sprintf("Rig registry entry for %s", rigName)
		c, s, u, seedErr := createOrSkipConfigBead(bd, slug, beads.ConfigCategoryRigRegistry,
			rigScope, "", "", metadata, desc)
		if seedErr != nil {
			return created, skipped, updated, fmt.Errorf("seeding rig %s: %w", rigName, seedErr)
		}
		created += c
		skipped += s
		updated += u

		// 2. Create per-rig config bead from rig/config.json (if it exists)
		rigConfigPath := filepath.Join(townRoot, rigName, "config.json")
		if _, statErr := os.Stat(rigConfigPath); statErr == nil {
			rigCfg, loadErr := config.LoadRigConfig(rigConfigPath)
			if loadErr != nil {
				fmt.Printf("  %s Skipping rigcfg for %s: %v\n", style.Warning.Render("!"), rigName, loadErr)
				continue
			}

			rigcfgSlug := "rigcfg-" + townName + "-" + rigName
			rigcfgMetadata := map[string]interface{}{
				"type":    rigCfg.Type,
				"name":    rigCfg.Name,
				"git_url": rigCfg.GitURL,
			}
			if rigCfg.LocalRepo != "" {
				rigcfgMetadata["local_repo"] = rigCfg.LocalRepo
			}
			if rigCfg.Beads != nil {
				rigcfgMetadata["beads"] = map[string]interface{}{
					"prefix": rigCfg.Beads.Prefix,
				}
			}

			rigcfgDesc := fmt.Sprintf("Rig config for %s", rigName)
			c, s, u, seedErr = createOrSkipConfigBead(bd, rigcfgSlug, beads.ConfigCategoryRigRegistry,
				rigScope, "", "", rigcfgMetadata, rigcfgDesc)
			if seedErr != nil {
				return created, skipped, updated, fmt.Errorf("seeding rigcfg %s: %w", rigName, seedErr)
			}
			created += c
			skipped += s
			updated += u
		}
	}

	return created, skipped, updated, nil
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

// seedAgentPresetBeads creates config beads for agent presets.
// For each built-in preset, creates a bead with the preset's configuration.
// Also creates a role-agents mapping bead with default role-to-agent assignments.
func seedAgentPresetBeads(bd *beads.Beads) (created, skipped, updated int, err error) {
	// Seed each built-in agent preset
	for _, name := range config.ListAgentPresets() {
		preset := config.GetAgentPresetByName(name)
		if preset == nil {
			continue
		}

		// Marshal preset to generic map for metadata storage
		presetJSON, marshalErr := json.Marshal(preset)
		if marshalErr != nil {
			return created, skipped, updated, fmt.Errorf("marshaling preset %s: %w", name, marshalErr)
		}

		var metadata map[string]interface{}
		if unmarshalErr := json.Unmarshal(presetJSON, &metadata); unmarshalErr != nil {
			return created, skipped, updated, fmt.Errorf("parsing preset %s: %w", name, unmarshalErr)
		}

		slug := "agent-" + name
		desc := fmt.Sprintf("Agent preset: %s", name)
		c, s, u, seedErr := createOrSkipConfigBead(bd, slug, beads.ConfigCategoryAgentPreset,
			"*", "", "", metadata, desc)
		if seedErr != nil {
			return created, skipped, updated, fmt.Errorf("creating agent preset bead for %s: %w", name, seedErr)
		}
		created += c
		skipped += s
		updated += u
	}

	// Create default role-agents mapping bead
	roleAgents := map[string]interface{}{
		"role_agents": map[string]string{
			"mayor":    "claude-opus",
			"deacon":   "claude-haiku",
			"witness":  "claude-haiku",
			"refinery": "claude-sonnet",
			"polecat":  "claude-sonnet",
			"crew":     "claude-sonnet",
		},
	}
	c, s, u, seedErr := createOrSkipConfigBead(bd, "role-agents-global", beads.ConfigCategoryAgentPreset,
		"*", "", "", roleAgents, "Default role-agent mappings")
	if seedErr != nil {
		return created, skipped, updated, fmt.Errorf("creating role-agents bead: %w", seedErr)
	}
	created += c
	skipped += s
	updated += u

	return created, skipped, updated, nil
}

// seedSlackBeads creates a config bead for Slack routing from settings/slack.json.
// Secrets (bot_token, app_token) are intentionally excluded from the bead metadata.
// They stay in environment variables (SLACK_BOT_TOKEN, SLACK_APP_TOKEN).
func seedSlackBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	slackPath := filepath.Join(townRoot, "settings", "slack.json")
	data, readErr := os.ReadFile(slackPath) //nolint:gosec // G304: path is constructed internally
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// No slack.json exists - skip silently
			fmt.Printf("  - Skipped slack-routing (no settings/slack.json found)\n")
			return 0, 1, 0, nil
		}
		return 0, 0, 0, fmt.Errorf("reading slack config: %w", readErr)
	}

	var slackMap map[string]interface{}
	if err := json.Unmarshal(data, &slackMap); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing slack config: %w", err)
	}

	// Remove secrets: bot_token and app_token must never go in beads
	delete(slackMap, "bot_token")
	delete(slackMap, "app_token")

	return createOrSkipConfigBead(bd, "slack-routing", beads.ConfigCategorySlackRouting,
		"*", "", "", slackMap, "Global Slack routing configuration")
}

// seedMessagingBeads creates a config bead for messaging configuration.
// Reads from config/messaging.json, or uses defaults if the file doesn't exist.
func seedMessagingBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	configPath := config.MessagingConfigPath(townRoot)
	msgConfig, loadErr := config.LoadOrCreateMessagingConfig(configPath)
	if loadErr != nil {
		return 0, 0, 0, fmt.Errorf("loading messaging config: %w", loadErr)
	}

	// Marshal to generic map for bead metadata storage
	msgJSON, err := json.Marshal(msgConfig)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshaling messaging config: %w", err)
	}

	var msgMap map[string]interface{}
	if err := json.Unmarshal(msgJSON, &msgMap); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing messaging config: %w", err)
	}

	return createOrSkipConfigBead(bd, "messaging", beads.ConfigCategoryMessaging,
		"*", "", "", msgMap, "Global messaging configuration")
}

// seedEscalationBeads creates a config bead for escalation configuration.
// Reads from settings/escalation.json, or uses defaults if the file doesn't exist.
func seedEscalationBeads(bd *beads.Beads, townRoot string) (created, skipped, updated int, err error) {
	configPath := config.EscalationConfigPath(townRoot)
	escConfig, loadErr := config.LoadOrCreateEscalationConfig(configPath)
	if loadErr != nil {
		return 0, 0, 0, fmt.Errorf("loading escalation config: %w", loadErr)
	}

	// Marshal to generic map for bead metadata storage
	escJSON, err := json.Marshal(escConfig)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshaling escalation config: %w", err)
	}

	var escMap map[string]interface{}
	if err := json.Unmarshal(escJSON, &escMap); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing escalation config: %w", err)
	}

	return createOrSkipConfigBead(bd, "escalation", beads.ConfigCategoryEscalation,
		"*", "", "", escMap, "Global escalation configuration")
}

