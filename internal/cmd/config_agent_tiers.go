package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Flags for 'gt config agent tiers set'
var (
	tiersSetAgent       string
	tiersSetDescription string
	tiersSetSelection   string
	tiersSetFallback    string
)

var configAgentTiersCmd = &cobra.Command{
	Use:   "tiers",
	Short: "Manage agent tier configuration",
	Long: `Manage the multi-tier agent routing system.

Agent tiers map capability levels to ordered agent lists, enabling automatic
agent selection based on task complexity and role.

Commands:
  gt config agent tiers init               Initialize default tier configuration
  gt config agent tiers show               Show current tier configuration
  gt config agent tiers set <name>         Create or update a tier
  gt config agent tiers remove <name>      Remove a tier
  gt config agent tiers set-role <role> <tier>  Map a role to a tier
  gt config agent tiers add-agent <tier> <agent>  Append agent to tier
  gt config agent tiers remove-agent <tier> <agent>  Remove agent from tier
  gt config agent tiers set-order <tier1> <tier2> ...  Set tier ordering`,
	RunE: requireSubcommand,
}

var configAgentTiersInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize default agent tier configuration",
	Long: `Initialize default agent tier configuration in town settings.

Creates a 4-tier default configuration (small, medium, large, reasoning)
with default role mappings. Only writes if agent_tiers is not already present.

Examples:
  gt config agent tiers init`,
	Args: cobra.NoArgs,
	RunE: runConfigAgentTiersInit,
}

var configAgentTiersShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current agent tier configuration",
	Long: `Show the current agent tier configuration.

Displays all tiers, their agents, selection strategies, role defaults,
and the tier ordering from lowest to highest capability.

Examples:
  gt config agent tiers show`,
	Args: cobra.NoArgs,
	RunE: runConfigAgentTiersShow,
}

var configAgentTiersSetCmd = &cobra.Command{
	Use:   "set <tier-name>",
	Short: "Create or update a tier",
	Long: `Create or update a tier in the agent tier configuration.

Creates the tier if it doesn't exist, updates if it does.
New tiers are appended to TierOrder if not already present.

Flags:
  --agent <name>               Set the agent list (single agent; replaces existing)
  --description <text>         Set tier description
  --selection <priority|round-robin>  Set selection strategy
  --fallback=<true|false>      Enable or disable fallback to next tier

Examples:
  gt config agent tiers set myTier --agent claude-sonnet --description "My tier"
  gt config agent tiers set large --selection round-robin
  gt config agent tiers set reasoning --fallback=false`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAgentTiersSet,
}

var configAgentTiersRemoveCmd = &cobra.Command{
	Use:   "remove <tier-name>",
	Short: "Remove a tier",
	Long: `Remove a tier from the agent tier configuration.

Removes the tier from the Tiers map, TierOrder, and any RoleDefaults
entries that reference it. Errors if the tier does not exist.

Examples:
  gt config agent tiers remove myTier`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAgentTiersRemove,
}

var configAgentTiersSetRoleCmd = &cobra.Command{
	Use:   "set-role <role> <tier-name>",
	Short: "Map a role to a tier",
	Long: `Map a role to a tier in the agent tier configuration.

Sets the default tier for a role in RoleDefaults. The tier must exist.
Overwrites any existing mapping for the role.

Examples:
  gt config agent tiers set-role polecat medium
  gt config agent tiers set-role mayor large`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigAgentTiersSetRole,
}

var configAgentTiersAddAgentCmd = &cobra.Command{
	Use:   "add-agent <tier-name> <agent-name>",
	Short: "Append an agent to a tier",
	Long: `Append an agent to a tier's agent list.

The tier must exist. The agent must not already be in the tier's list.
Warns if the agent name is not a known preset (custom agents may be
registered later).

Examples:
  gt config agent tiers add-agent medium claude-sonnet
  gt config agent tiers add-agent large claude-opus`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigAgentTiersAddAgent,
}

var configAgentTiersRemoveAgentCmd = &cobra.Command{
	Use:   "remove-agent <tier-name> <agent-name>",
	Short: "Remove an agent from a tier",
	Long: `Remove an agent from a tier's agent list.

The tier and agent must exist. Errors if removing would leave the tier
with an empty agent list.

Examples:
  gt config agent tiers remove-agent medium claude-haiku`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigAgentTiersRemoveAgent,
}

var configAgentTiersSetOrderCmd = &cobra.Command{
	Use:   "set-order <tier1> <tier2> ...",
	Short: "Set the tier ordering",
	Long: `Set the capability ordering of tiers from lowest to highest.

All tier names in the Tiers map must appear in the list, and all names
in the list must exist in Tiers. The first tier is the lowest capability
tier; fallback escalates upward through the list.

Examples:
  gt config agent tiers set-order small medium large reasoning`,
	Args: cobra.MinimumNArgs(1),
	RunE: runConfigAgentTiersSetOrder,
}

func runConfigAgentTiersInit(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers != nil {
		fmt.Println("Agent tiers already configured. Use 'gt config agent tiers show' to view.")
		return nil
	}

	townSettings.AgentTiers = config.DefaultAgentTierConfig()

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Agent tiers initialized with defaults"))
	printTiersSummary(townSettings.AgentTiers)
	return nil
}

func runConfigAgentTiersShow(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil {
		fmt.Println("No agent tiers configured. Run 'gt config agent tiers init' to set up defaults.")
		return nil
	}

	printTiersSummary(townSettings.AgentTiers)
	return nil
}

func printTiersSummary(tc *config.AgentTierConfig) {
	fmt.Printf("%s\n\n", style.Bold.Render("Agent Tier Configuration"))

	fmt.Printf("%s (lowest → highest):\n", style.Bold.Render("Tier Order"))
	if len(tc.TierOrder) == 0 {
		fmt.Println("  (none)")
	} else {
		fmt.Printf("  %s\n", strings.Join(tc.TierOrder, " → "))
	}
	fmt.Println()

	fmt.Printf("%s:\n", style.Bold.Render("Tiers"))
	for _, name := range tc.TierNames() {
		tier := tc.Tiers[name]
		if tier == nil {
			continue
		}
		selection := tier.Selection
		if selection == "" {
			selection = "priority"
		}
		fallbackStr := "yes"
		if !tier.Fallback {
			fallbackStr = "no"
		}
		fmt.Printf("  %s\n", style.Bold.Render(name))
		fmt.Printf("    Description: %s\n", tier.Description)
		fmt.Printf("    Agents:      %s\n", strings.Join(tier.Agents, ", "))
		fmt.Printf("    Selection:   %s\n", selection)
		fmt.Printf("    Fallback:    %s\n", fallbackStr)
	}
	fmt.Println()

	fmt.Printf("%s:\n", style.Bold.Render("Role Defaults"))
	if len(tc.RoleDefaults) == 0 {
		fmt.Println("  (none)")
	} else {
		// Print in a stable order
		roles := make([]string, 0, len(tc.RoleDefaults))
		for role := range tc.RoleDefaults {
			roles = append(roles, role)
		}
		// Sort for stability
		for i := 0; i < len(roles); i++ {
			for j := i + 1; j < len(roles); j++ {
				if roles[i] > roles[j] {
					roles[i], roles[j] = roles[j], roles[i]
				}
			}
		}
		for _, role := range roles {
			fmt.Printf("  %-12s → %s\n", role, tc.RoleDefaults[role])
		}
	}
}

func runConfigAgentTiersSet(cmd *cobra.Command, args []string) error {
	tierName := args[0]
	if err := validateIdentifier("tier name", tierName, ""); err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil {
		townSettings.AgentTiers = &config.AgentTierConfig{
			Tiers:    make(map[string]*config.AgentTier),
			TierOrder: []string{},
		}
	}
	if townSettings.AgentTiers.Tiers == nil {
		townSettings.AgentTiers.Tiers = make(map[string]*config.AgentTier)
	}

	// Get or create tier
	tier, exists := townSettings.AgentTiers.Tiers[tierName]
	if !exists {
		tier = &config.AgentTier{
			Selection: "priority",
			Fallback:  true,
		}
	}

	// Apply flags
	if tiersSetAgent != "" {
		if err := validateIdentifier("agent name", tiersSetAgent, "."); err != nil {
			return err
		}
		// Warn if not a known preset
		if !isKnownAgentPreset(tiersSetAgent) {
			fmt.Printf("Warning: agent %q is not a known preset (may be a custom agent)\n", tiersSetAgent)
		}
		tier.Agents = []string{tiersSetAgent}
	}
	if tiersSetDescription != "" {
		tier.Description = tiersSetDescription
	}
	if tiersSetSelection != "" {
		if tiersSetSelection != "priority" && tiersSetSelection != "round-robin" {
			return fmt.Errorf("invalid selection strategy %q (must be \"priority\" or \"round-robin\")", tiersSetSelection)
		}
		tier.Selection = tiersSetSelection
	}
	if cmd.Flags().Changed("fallback") {
		fb, err := parseBool(tiersSetFallback)
		if err != nil {
			return fmt.Errorf("invalid fallback value: %w", err)
		}
		tier.Fallback = fb
	}

	townSettings.AgentTiers.Tiers[tierName] = tier

	// Append to TierOrder if new
	if !exists {
		inOrder := false
		for _, n := range townSettings.AgentTiers.TierOrder {
			if n == tierName {
				inOrder = true
				break
			}
		}
		if !inOrder {
			townSettings.AgentTiers.TierOrder = append(townSettings.AgentTiers.TierOrder, tierName)
		}
	}

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	if exists {
		fmt.Printf("Updated tier %s\n", style.Bold.Render(tierName))
	} else {
		fmt.Printf("Created tier %s\n", style.Bold.Render(tierName))
	}
	return nil
}

func runConfigAgentTiersRemove(cmd *cobra.Command, args []string) error {
	tierName := args[0]
	if err := validateIdentifier("tier name", tierName, ""); err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil || townSettings.AgentTiers.Tiers == nil {
		return fmt.Errorf("tier %q not found (no tiers configured)", tierName)
	}
	if _, ok := townSettings.AgentTiers.Tiers[tierName]; !ok {
		return fmt.Errorf("tier %q not found", tierName)
	}

	delete(townSettings.AgentTiers.Tiers, tierName)

	// Remove from TierOrder
	filtered := townSettings.AgentTiers.TierOrder[:0]
	for _, n := range townSettings.AgentTiers.TierOrder {
		if n != tierName {
			filtered = append(filtered, n)
		}
	}
	townSettings.AgentTiers.TierOrder = filtered

	// Remove from RoleDefaults
	for role, tn := range townSettings.AgentTiers.RoleDefaults {
		if tn == tierName {
			delete(townSettings.AgentTiers.RoleDefaults, role)
		}
	}

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Removed tier %s\n", style.Bold.Render(tierName))
	return nil
}

func runConfigAgentTiersSetRole(cmd *cobra.Command, args []string) error {
	role := args[0]
	tierName := args[1]
	if err := validateIdentifier("role name", role, "/"); err != nil {
		return err
	}
	if err := validateIdentifier("tier name", tierName, ""); err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil || !townSettings.AgentTiers.HasTier(tierName) {
		return fmt.Errorf("tier %q not found; use 'gt config agent tiers show' to list tiers", tierName)
	}

	if townSettings.AgentTiers.RoleDefaults == nil {
		townSettings.AgentTiers.RoleDefaults = make(map[string]string)
	}
	townSettings.AgentTiers.RoleDefaults[role] = tierName

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Role %s → tier %s\n", style.Bold.Render(role), style.Bold.Render(tierName))
	return nil
}

func runConfigAgentTiersAddAgent(cmd *cobra.Command, args []string) error {
	tierName := args[0]
	agentName := args[1]
	if err := validateIdentifier("tier name", tierName, ""); err != nil {
		return err
	}
	if err := validateIdentifier("agent name", agentName, "."); err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil || !townSettings.AgentTiers.HasTier(tierName) {
		return fmt.Errorf("tier %q not found", tierName)
	}

	tier := townSettings.AgentTiers.Tiers[tierName]
	for _, a := range tier.Agents {
		if a == agentName {
			return fmt.Errorf("agent %q already in tier %q", agentName, tierName)
		}
	}

	if !isKnownAgentPreset(agentName) {
		fmt.Printf("Warning: agent %q is not a known preset (may be a custom agent)\n", agentName)
	}

	tier.Agents = append(tier.Agents, agentName)

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Added agent %s to tier %s\n", style.Bold.Render(agentName), style.Bold.Render(tierName))
	return nil
}

func runConfigAgentTiersRemoveAgent(cmd *cobra.Command, args []string) error {
	tierName := args[0]
	agentName := args[1]
	if err := validateIdentifier("tier name", tierName, ""); err != nil {
		return err
	}
	if err := validateIdentifier("agent name", agentName, "."); err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil || !townSettings.AgentTiers.HasTier(tierName) {
		return fmt.Errorf("tier %q not found", tierName)
	}

	tier := townSettings.AgentTiers.Tiers[tierName]
	idx := -1
	for i, a := range tier.Agents {
		if a == agentName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("agent %q not found in tier %q", agentName, tierName)
	}
	if len(tier.Agents) == 1 {
		return fmt.Errorf("cannot remove %q: would leave tier %q with no agents", agentName, tierName)
	}

	tier.Agents = append(tier.Agents[:idx], tier.Agents[idx+1:]...)

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Removed agent %s from tier %s\n", style.Bold.Render(agentName), style.Bold.Render(tierName))
	return nil
}

func runConfigAgentTiersSetOrder(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.AgentTiers == nil {
		return fmt.Errorf("no agent tiers configured; run 'gt config agent tiers init' first")
	}

	// Validate tier name format before existence checks
	for _, name := range args {
		if err := validateIdentifier("tier name", name, ""); err != nil {
			return err
		}
	}

	// Validate: all args must exist in Tiers
	for _, name := range args {
		if !townSettings.AgentTiers.HasTier(name) {
			return fmt.Errorf("tier %q not found in configuration", name)
		}
	}

	// Validate: all tiers in Tiers must appear in args
	for name := range townSettings.AgentTiers.Tiers {
		found := false
		for _, a := range args {
			if a == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("tier %q exists in configuration but is missing from the order list", name)
		}
	}

	townSettings.AgentTiers.TierOrder = args

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Tier order set: %s\n", strings.Join(args, " → "))
	return nil
}

// isKnownAgentPreset returns true if name is a known built-in agent preset.
func isKnownAgentPreset(name string) bool {
	for _, preset := range config.ListAgentPresets() {
		if preset == name {
			return true
		}
	}
	return false
}

// validateIdentifier checks that a name is non-empty and contains only
// alphanumeric characters, hyphens, and underscores (plus any extra allowed chars).
func validateIdentifier(kind, name string, extraAllowed string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", kind)
	}
	for _, c := range name {
		valid := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !valid && extraAllowed != "" {
			valid = strings.ContainsRune(extraAllowed, c)
		}
		if !valid {
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				return fmt.Errorf("%s %q must not contain whitespace", kind, name)
			}
			return fmt.Errorf("%s %q contains invalid character %q (use letters, digits, hyphens, underscores%s)", kind, name, string(c), extraAllowedHint(extraAllowed))
		}
	}
	return nil
}

// extraAllowedHint returns a human-readable hint for extra allowed characters.
func extraAllowedHint(extra string) string {
	if extra == "" {
		return ""
	}
	hints := make([]string, 0, len(extra))
	for _, c := range extra {
		switch c {
		case '.':
			hints = append(hints, "dots")
		case '/':
			hints = append(hints, "slashes")
		default:
			hints = append(hints, string(c))
		}
	}
	return ", " + strings.Join(hints, ", ")
}

func init() {
	// Flags for 'set'
	configAgentTiersSetCmd.Flags().StringVar(&tiersSetAgent, "agent", "", "Agent name (replaces existing list)")
	configAgentTiersSetCmd.Flags().StringVar(&tiersSetDescription, "description", "", "Tier description")
	configAgentTiersSetCmd.Flags().StringVar(&tiersSetSelection, "selection", "", "Selection strategy (priority or round-robin)")
	configAgentTiersSetCmd.Flags().StringVar(&tiersSetFallback, "fallback", "", "Enable fallback (true or false)")

	// Wire up subcommands
	configAgentTiersCmd.AddCommand(configAgentTiersInitCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersShowCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersSetCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersRemoveCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersSetRoleCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersAddAgentCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersRemoveAgentCmd)
	configAgentTiersCmd.AddCommand(configAgentTiersSetOrderCmd)
}
