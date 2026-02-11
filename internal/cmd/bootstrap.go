// Package cmd provides CLI commands for the gt tool.
// This file implements gt bootstrap — seeds a new Dolt DB for K8s namespaces.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	bootstrapTownName string
	bootstrapRigs     []string // "name:prefix:git_url" format
	bootstrapAgents   []string // "rig:role:name" format
	bootstrapDryRun   bool
	bootstrapForce    bool
)

var bootstrapCmd = &cobra.Command{
	Use:     "bootstrap",
	GroupID: GroupConfig,
	Short:   "Seed a new Dolt DB for K8s namespace deployment",
	Long: `Bootstrap seeds a fresh Dolt database with the minimum config beads
required for a functioning gastown namespace.

This is the first command to run after 'gt connect' on a new K8s namespace.
It creates:
  - Town identity config bead
  - Rig beads and route beads for each configured rig
  - Config beads: hooks, roles, agent presets, messaging, escalation

Agent beads are NOT created by bootstrap — they are created by the
controller when it reconciles (bead-first pattern), or by 'gt rig register'
when adding agents to a rig. The --agent flag exists only for testing.

Examples:
  # Bootstrap with town name and rigs
  gt bootstrap --town gastown-next \
    --rig "beads:bd:https://github.com/org/beads.git" \
    --rig "gastown:gt:https://github.com/org/gastown.git"

  # Dry run to see what would be created
  gt bootstrap --town gastown-next --dry-run

The command is idempotent — safe to run multiple times.
Existing beads are skipped unless --force is specified.`,
	RunE: runBootstrap,
}

func init() {
	bootstrapCmd.Flags().StringVar(&bootstrapTownName, "town", "", "Town name (required)")
	bootstrapCmd.Flags().StringSliceVar(&bootstrapRigs, "rig", nil,
		`Rig to register as "name:prefix:git_url" (repeatable)`)
	bootstrapCmd.Flags().StringSliceVar(&bootstrapAgents, "agent", nil,
		`[testing only] Pre-create agent bead as "rig:role:name" (normally created by controller)`)
	bootstrapCmd.Flags().BoolVar(&bootstrapDryRun, "dry-run", false, "Show what would be created")
	bootstrapCmd.Flags().BoolVar(&bootstrapForce, "force", false, "Overwrite existing beads")
	_ = bootstrapCmd.MarkFlagRequired("town")

	rootCmd.AddCommand(bootstrapCmd)
}

type bootstrapStats struct {
	created int
	skipped int
	updated int
}

func (s *bootstrapStats) add(c, sk, u int) {
	s.created += c
	s.skipped += sk
	s.updated += u
}

func runBootstrap(_ *cobra.Command, _ []string) error {
	// Resolve workspace — may not fully exist yet in K8s
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		// In K8s, workspace might be minimal. Use CWD.
		townRoot, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine workspace: %w", err)
		}
	}

	bd := beads.New(townRoot)
	stats := &bootstrapStats{}

	fmt.Printf("%s Bootstrapping town %s\n\n", style.Info.Render("⚙"), style.Bold.Render(bootstrapTownName))

	// Phase 1: Town identity
	fmt.Println(style.Bold.Render("Phase 1: Town identity"))
	if err := bootstrapTownIdentity(bd, townRoot, stats); err != nil {
		return fmt.Errorf("town identity: %w", err)
	}
	fmt.Println()

	// Phase 2: Rigs
	if len(bootstrapRigs) > 0 {
		fmt.Println(style.Bold.Render("Phase 2: Rig registration"))
		if err := bootstrapRigBeads(bd, townRoot, stats); err != nil {
			return fmt.Errorf("rig registration: %w", err)
		}
		fmt.Println()
	}

	// Phase 3: Agents
	if len(bootstrapAgents) > 0 {
		fmt.Println(style.Bold.Render("Phase 3: Agent beads"))
		if err := bootstrapAgentBeads(bd, stats); err != nil {
			return fmt.Errorf("agent beads: %w", err)
		}
		fmt.Println()
	}

	// Phase 4: Config beads (hooks, roles, agent presets, messaging, escalation)
	fmt.Println(style.Bold.Render("Phase 4: Config beads"))
	if err := bootstrapConfigBeads(bd, townRoot, stats); err != nil {
		return fmt.Errorf("config beads: %w", err)
	}
	fmt.Println()

	// Summary
	if bootstrapDryRun {
		fmt.Printf("%s Dry run complete: would create %d, would skip %d, would update %d\n",
			style.Info.Render("ℹ"), stats.created, stats.skipped, stats.updated)
	} else {
		fmt.Printf("%s Bootstrap complete: created %d, skipped %d, updated %d\n",
			style.Success.Render("✓"), stats.created, stats.skipped, stats.updated)
	}

	return nil
}

// bootstrapTownIdentity creates the town identity config bead and local files.
func bootstrapTownIdentity(bd *beads.Beads, townRoot string, stats *bootstrapStats) error {
	// Create mayor/town.json locally
	townConfigDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(townConfigDir, 0o755); err != nil {
		return fmt.Errorf("creating mayor dir: %w", err)
	}

	townConfigPath := filepath.Join(townConfigDir, "town.json")
	if _, err := os.Stat(townConfigPath); os.IsNotExist(err) {
		tc := &config.TownConfig{
			Type:      "town",
			Version:   1,
			Name:      bootstrapTownName,
			CreatedAt: time.Now(),
		}
		data, marshalErr := json.MarshalIndent(tc, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshaling town config: %w", marshalErr)
		}
		if !bootstrapDryRun {
			if err := os.WriteFile(townConfigPath, data, 0o644); err != nil {
				return fmt.Errorf("writing town config: %w", err)
			}
		}
		fmt.Printf("  %s mayor/town.json\n", actionLabel("Created"))
	} else {
		fmt.Printf("  - Skipped mayor/town.json (exists)\n")
	}

	// Create town identity config bead
	metadata := map[string]interface{}{
		"type":       "town",
		"version":    1,
		"name":       bootstrapTownName,
		"created_at": time.Now(),
	}
	c, s, u, err := bootstrapConfigBead(bd, "town-"+bootstrapTownName,
		beads.ConfigCategoryIdentity, bootstrapTownName, "", "",
		metadata, "Town identity: "+bootstrapTownName)
	if err != nil {
		return err
	}
	stats.add(c, s, u)

	return nil
}

// bootstrapRigBeads registers rigs and creates rig beads.
func bootstrapRigBeads(bd *beads.Beads, townRoot string, stats *bootstrapStats) error {
	// Load or create rigs.json
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{
			Version: 1,
			Rigs:    make(map[string]config.RigEntry),
		}
	}

	for _, rigSpec := range bootstrapRigs {
		parts := strings.SplitN(rigSpec, ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid rig spec %q — expected name:prefix:git_url", rigSpec)
		}
		name, prefix, gitURL := parts[0], parts[1], parts[2]

		// Add to rigs.json
		if _, exists := rigsConfig.Rigs[name]; !exists {
			rigsConfig.Rigs[name] = config.RigEntry{
				GitURL:  gitURL,
				AddedAt: time.Now(),
				BeadsConfig: &config.BeadsConfig{
					Prefix: prefix,
				},
			}
		}

		// Create rig directory
		rigDir := filepath.Join(townRoot, name)
		if !bootstrapDryRun {
			_ = os.MkdirAll(rigDir, 0o755)
		}

		// Create rig bead (type=rig) with labels
		rigID := "hq-" + prefix + "-rig-" + name
		labels := []string{
			"gt:rig",
			fmt.Sprintf("prefix:%s", prefix),
			fmt.Sprintf("git_url:%s", gitURL),
			"state:active",
		}
		title := fmt.Sprintf("Rig: %s", name)
		desc := beads.FormatRigDescription(name, &beads.RigFields{
			Repo:   gitURL,
			Prefix: prefix,
			State:  "active",
		})

		c, s, u, err := bootstrapBead(bd, rigID, title, desc, labels, "rig")
		if err != nil {
			return fmt.Errorf("creating rig bead for %s: %w", name, err)
		}
		stats.add(c, s, u)

		// Create rig registry config bead
		rigScope := bootstrapTownName + "/" + name
		rigMeta := map[string]interface{}{
			"git_url":  gitURL,
			"added_at": time.Now(),
			"beads":    map[string]interface{}{"prefix": prefix},
		}
		c, s, u, err = bootstrapConfigBead(bd, "rig-"+bootstrapTownName+"-"+name,
			beads.ConfigCategoryRigRegistry, rigScope, "", "",
			rigMeta, "Rig registry: "+name)
		if err != nil {
			return fmt.Errorf("creating rig config bead for %s: %w", name, err)
		}
		stats.add(c, s, u)

		// Create route bead (type=route) — enables prefix-based routing via daemon
		routeID := "hq-route-" + prefix
		routeTitle := fmt.Sprintf("%s- → %s", prefix, name)
		routeDesc := fmt.Sprintf("Route for prefix %s- to path %s", prefix, name)
		routeLabels := []string{
			"prefix:" + prefix,
			"path:" + name,
		}
		c, s, u, err = bootstrapBead(bd, routeID, routeTitle, routeDesc, routeLabels, "route")
		if err != nil {
			return fmt.Errorf("creating route bead for %s: %w", name, err)
		}
		stats.add(c, s, u)
	}

	// Create hq- route for town root (hq-* beads live in town-level db)
	hqRouteID := "hq-route-hq"
	hqLabels := []string{"prefix:hq", "path:."}
	c, s, u, err := bootstrapBead(bd, hqRouteID, "hq- → town root",
		"Route for prefix hq- to town root", hqLabels, "route")
	if err != nil {
		return fmt.Errorf("creating hq route bead: %w", err)
	}
	stats.add(c, s, u)

	// Save rigs.json
	if !bootstrapDryRun {
		if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
			return fmt.Errorf("saving rigs config: %w", err)
		}
	}
	fmt.Printf("  %s mayor/rigs.json (%d rigs)\n", actionLabel("Updated"), len(rigsConfig.Rigs))

	return nil
}

// bootstrapAgentBeads creates agent beads for K8s-managed agents.
func bootstrapAgentBeads(bd *beads.Beads, stats *bootstrapStats) error {
	// Build rig name→prefix map from --rig flags
	rigPrefixes := make(map[string]string)
	for _, rigSpec := range bootstrapRigs {
		parts := strings.SplitN(rigSpec, ":", 3)
		if len(parts) >= 2 {
			rigPrefixes[parts[0]] = parts[1]
		}
	}

	for _, agentSpec := range bootstrapAgents {
		parts := strings.SplitN(agentSpec, ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid agent spec %q — expected rig:role:name", agentSpec)
		}
		rigName, role, name := parts[0], parts[1], parts[2]

		// Look up rig prefix (default to "gt" if rig not in --rig flags)
		prefix := rigPrefixes[rigName]
		if prefix == "" {
			prefix = "gt"
		}

		// Use canonical ID generation functions based on role
		var agentID string
		switch role {
		case "mayor":
			agentID = beads.MayorBeadIDTown()
		case "deacon":
			agentID = beads.DeaconBeadIDTown()
		case "dog":
			agentID = beads.DogBeadIDTown(name)
		case "polecat":
			agentID = beads.PolecatBeadIDWithPrefix(prefix, rigName, name)
		case "crew":
			agentID = beads.CrewBeadIDWithPrefix(prefix, rigName, name)
		case "witness":
			agentID = beads.WitnessBeadIDWithPrefix(prefix, rigName)
		case "refinery":
			agentID = beads.RefineryBeadIDWithPrefix(prefix, rigName)
		default:
			agentID = beads.AgentBeadIDWithPrefix(prefix, rigName, role, name)
		}
		title := fmt.Sprintf("%s/%s/%s", rigName, role, name)

		if bootstrapDryRun {
			fmt.Printf("  %s %s (gt:agent, execution_target:k8s)\n",
				style.Info.Render("+"), agentID)
			stats.created++
			continue
		}

		// Check if already exists
		existing, _ := bd.Show(agentID)
		if existing != nil {
			fmt.Printf("  - Skipped %s (exists)\n", agentID)
			stats.skipped++
			continue
		}

		// Use CreateWithID + labels instead of CreateOrReopenAgentBead,
		// because the latter tries to resolve a local .beads directory
		// which doesn't exist when bootstrapping a remote daemon.
		description := fmt.Sprintf("role_type: %s\nrig: %s\nagent_state: spawning", role, rigName)
		_, err := bd.CreateWithID(agentID, beads.CreateOptions{
			Title:       title,
			Type:        "agent",
			Description: description,
		})
		if err != nil {
			return fmt.Errorf("creating agent bead %s: %w", agentID, err)
		}

		// Add required labels for controller detection + structured metadata
		labels := []string{
			"gt:agent",
			"execution_target:k8s",
			"rig:" + rigName,
			"role:" + role,
			"agent:" + name,
		}
		for _, label := range labels {
			if addErr := bd.AddLabel(agentID, label); addErr != nil {
				fmt.Printf("  %s Could not add label %s to %s: %v\n",
					style.Warning.Render("!"), label, agentID, addErr)
			}
		}

		fmt.Printf("  %s %s (gt:agent, execution_target:k8s)\n",
			style.Success.Render("✓"), agentID)
		stats.created++
	}

	return nil
}

// bootstrapConfigBeads seeds essential config beads using embedded defaults.
// This works without local config files — it uses the same embedded templates
// as config_seed.go but doesn't require town.json/rigs.json/etc to exist.
func bootstrapConfigBeads(bd *beads.Beads, townRoot string, stats *bootstrapStats) error {
	// Hooks (from embedded templates)
	c, s, u, err := bootstrapHookBeads(bd)
	if err != nil {
		return fmt.Errorf("hook beads: %w", err)
	}
	stats.add(c, s, u)

	// Role definitions (from built-in defaults)
	c, s, u, err = bootstrapRoleBeads(bd)
	if err != nil {
		return fmt.Errorf("role beads: %w", err)
	}
	stats.add(c, s, u)

	// NOTE: Agent presets (claude, gemini, codex, etc.) are NOT seeded here.
	// They are hardcoded built-ins in config/agents.go. The configbeads agent
	// preset path exists but has no runtime callers in the daemon.

	// Messaging config (defaults)
	msgConfig := config.NewMessagingConfig()
	msgJSON, _ := json.Marshal(msgConfig)
	var msgMap map[string]interface{}
	_ = json.Unmarshal(msgJSON, &msgMap)
	c, s, u, err = bootstrapConfigBead(bd, "messaging", beads.ConfigCategoryMessaging,
		"*", "", "", msgMap, "Global messaging configuration")
	if err != nil {
		return fmt.Errorf("messaging bead: %w", err)
	}
	stats.add(c, s, u)

	// Escalation config (defaults)
	escConfig := config.NewEscalationConfig()
	escJSON, _ := json.Marshal(escConfig)
	var escMap map[string]interface{}
	_ = json.Unmarshal(escJSON, &escMap)
	c, s, u, err = bootstrapConfigBead(bd, "escalation", beads.ConfigCategoryEscalation,
		"*", "", "", escMap, "Global escalation configuration")
	if err != nil {
		return fmt.Errorf("escalation bead: %w", err)
	}
	stats.add(c, s, u)

	// Daemon patrol config (defaults)
	daemonConfig := config.NewDaemonPatrolConfig()
	daemonJSON, _ := json.Marshal(daemonConfig)
	var daemonMap map[string]interface{}
	_ = json.Unmarshal(daemonJSON, &daemonMap)
	c, s, u, err = bootstrapConfigBead(bd, "daemon-patrol", beads.ConfigCategoryDaemon,
		"*", "", "", daemonMap, "Global daemon patrol configuration")
	if err != nil {
		return fmt.Errorf("daemon patrol bead: %w", err)
	}
	stats.add(c, s, u)

	return nil
}

// bootstrapHookBeads creates hook config beads from embedded templates.
// Reuses the same diffing logic as seedHookBeads in config_seed.go.
func bootstrapHookBeads(bd *beads.Beads) (created, skipped, updated int, err error) {
	autoContent, err := claude.TemplateContent(claude.Autonomous)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reading autonomous template: %w", err)
	}
	interContent, err := claude.TemplateContent(claude.Interactive)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reading interactive template: %w", err)
	}

	var autoSettings, interSettings map[string]interface{}
	if err := json.Unmarshal(autoContent, &autoSettings); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing autonomous template: %w", err)
	}
	if err := json.Unmarshal(interContent, &interSettings); err != nil {
		return 0, 0, 0, fmt.Errorf("parsing interactive template: %w", err)
	}

	autoHooks := extractHooksMap(autoSettings)
	interHooks := extractHooksMap(interSettings)

	baseHooks := make(map[string]interface{})
	autoOnlyHooks := make(map[string]interface{})
	interOnlyHooks := make(map[string]interface{})

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
			baseHooks[name] = autoHooks[name]
		} else {
			if autoHooks[name] != nil {
				autoOnlyHooks[name] = autoHooks[name]
			}
			if interHooks[name] != nil {
				interOnlyHooks[name] = interHooks[name]
			}
		}
	}

	baseSettings := make(map[string]interface{})
	for k, v := range autoSettings {
		if k != "hooks" {
			baseSettings[k] = v
		}
	}
	if len(baseHooks) > 0 {
		baseSettings["hooks"] = baseHooks
	}

	c, s, u, err := bootstrapConfigBead(bd, "hooks-base", beads.ConfigCategoryClaudeHooks,
		"*", "", "", baseSettings, "Base Claude hooks shared by all roles")
	if err != nil {
		return 0, 0, 0, err
	}
	created += c
	skipped += s
	updated += u

	if len(autoOnlyHooks) > 0 {
		c, s, u, err = bootstrapConfigBead(bd, "hooks-polecat", beads.ConfigCategoryClaudeHooks,
			"*", "polecat", "", map[string]interface{}{"hooks": autoOnlyHooks},
			"Polecat-specific hook overrides")
		if err != nil {
			return created, skipped, updated, err
		}
		created += c
		skipped += s
		updated += u
	}

	if len(interOnlyHooks) > 0 {
		c, s, u, err = bootstrapConfigBead(bd, "hooks-crew", beads.ConfigCategoryClaudeHooks,
			"*", "crew", "", map[string]interface{}{"hooks": interOnlyHooks},
			"Crew-specific hook overrides")
		if err != nil {
			return created, skipped, updated, err
		}
		created += c
		skipped += s
		updated += u
	}

	return created, skipped, updated, nil
}

// bootstrapRoleBeads seeds built-in role definitions.
func bootstrapRoleBeads(bd *beads.Beads) (created, skipped, updated int, err error) {
	for _, roleName := range config.AllRoles() {
		def, loadErr := config.LoadBuiltinRoleDefinition(roleName)
		if loadErr != nil {
			continue // Skip roles that can't be loaded
		}

		metadata, marshalErr := roleDefToMetadata(def)
		if marshalErr != nil {
			continue
		}

		slug := "role-" + roleName
		desc := fmt.Sprintf("Role definition: %s (scope: %s)", roleName, def.Scope)
		c, s, u, seedErr := bootstrapConfigBead(bd, slug, beads.ConfigCategoryRoleDefinition,
			"*", "", "", metadata, desc)
		if seedErr != nil {
			return created, skipped, updated, fmt.Errorf("seeding role %s: %w", roleName, seedErr)
		}
		created += c
		skipped += s
		updated += u
	}
	return created, skipped, updated, nil
}

// bootstrapConfigBead creates or skips a config bead. Idempotent.
func bootstrapConfigBead(bd *beads.Beads, slug, category, rig, role, agent string,
	metadata interface{}, description string) (created, skipped, updated int, err error) {

	id := beads.ConfigBeadID(slug)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshaling %s: %w", slug, err)
	}

	existing, _, getErr := bd.GetConfigBead(id)
	if getErr != nil {
		return 0, 0, 0, fmt.Errorf("checking %s: %w", id, getErr)
	}

	if existing != nil {
		if bootstrapForce {
			if bootstrapDryRun {
				fmt.Printf("  %s Would update %s (%s)\n", style.Warning.Render("~"), id, description)
				return 0, 0, 1, nil
			}
			if err := bd.UpdateConfigMetadata(id, string(metadataJSON)); err != nil {
				return 0, 0, 0, fmt.Errorf("updating %s: %w", id, err)
			}
			fmt.Printf("  %s Updated %s (%s)\n", style.Success.Render("✓"), id, description)
			return 0, 0, 1, nil
		}
		fmt.Printf("  - Skipped %s (exists)\n", id)
		return 0, 1, 0, nil
	}

	if bootstrapDryRun {
		fmt.Printf("  %s Would create %s (%s)\n", style.Info.Render("+"), id, description)
		return 1, 0, 0, nil
	}

	fields := &beads.ConfigFields{
		Rig:      rig,
		Category: category,
		Metadata: string(metadataJSON),
	}
	if _, err := bd.CreateConfigBead(slug, fields, role, agent); err != nil {
		return 0, 0, 0, fmt.Errorf("creating %s: %w", id, err)
	}

	fmt.Printf("  %s Created %s (%s)\n", style.Success.Render("✓"), id, description)
	return 1, 0, 0, nil
}

// bootstrapBead creates a generic bead with labels. Idempotent.
func bootstrapBead(bd *beads.Beads, id, title, description string, labels []string, beadType string) (created, skipped, updated int, err error) {
	// Check if exists
	existing, showErr := bd.Show(id)
	if showErr == nil && existing != nil {
		if bootstrapForce {
			if bootstrapDryRun {
				fmt.Printf("  %s Would update %s\n", style.Warning.Render("~"), id)
				return 0, 0, 1, nil
			}
			// Update description
			if err := bd.Update(id, beads.UpdateOptions{Description: &description}); err != nil {
				return 0, 0, 0, fmt.Errorf("updating %s: %w", id, err)
			}
			fmt.Printf("  %s Updated %s\n", style.Success.Render("✓"), id)
			return 0, 0, 1, nil
		}
		fmt.Printf("  - Skipped %s (exists)\n", id)
		return 0, 1, 0, nil
	}

	if bootstrapDryRun {
		fmt.Printf("  %s Would create %s\n", style.Info.Render("+"), id)
		return 1, 0, 0, nil
	}

	// Create with beads API
	_, err = bd.CreateWithID(id, beads.CreateOptions{
		Title:       title,
		Type:        beadType,
		Description: description,
	})
	if err != nil {
		return 0, 0, 0, fmt.Errorf("creating %s: %w", id, err)
	}
	for _, label := range labels {
		if addErr := bd.AddLabel(id, label); addErr != nil {
			fmt.Printf("  Warning: could not add label %s to %s: %v\n", label, id, addErr)
		}
	}

	fmt.Printf("  %s Created %s\n", style.Success.Render("✓"), id)
	return 1, 0, 0, nil
}

func actionLabel(action string) string {
	if bootstrapDryRun {
		return style.Info.Render("+ Would create")
	}
	return style.Success.Render("✓ " + action)
}
