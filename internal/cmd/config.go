// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var configCmd = &cobra.Command{
	Use:     "config",
	GroupID: GroupConfig,
	Short:   "Manage Gas Town configuration",
	RunE:    requireSubcommand,
	Long: `Manage Gas Town configuration settings.

This command allows you to view and modify configuration settings
for your Gas Town workspace, including agent aliases and defaults.

Commands:
  gt config agent list              List all agents (built-in and custom)
  gt config agent get <name>         Show agent configuration
  gt config agent set <name> <cmd>   Set custom agent command
  gt config agent remove <name>      Remove custom agent
  gt config default-agent [name]     Get or set default agent`,
}

// Agent subcommands

var configAgentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	Long: `List all available agents (built-in and custom).

Shows all built-in agent presets (claude, gemini, codex) and any
custom agents defined in your town settings.

Examples:
  gt config agent list           # Text output
  gt config agent list --json    # JSON output`,
	RunE: runConfigAgentList,
}

var configAgentGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show agent configuration",
	Long: `Show the configuration for a specific agent.

Displays the full configuration for an agent, including command,
arguments, and other settings. Works for both built-in and custom agents.

Examples:
  gt config agent get claude
  gt config agent get my-custom-agent`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAgentGet,
}

var configAgentSetCmd = &cobra.Command{
	Use:   "set <name> <command>",
	Short: "Set custom agent command",
	Long: `Set a custom agent command in town settings.

This creates or updates a custom agent definition that overrides
or extends the built-in presets. The custom agent will be available
to all rigs in the town.

The command can include arguments. Use quotes if the command or
arguments contain spaces.

Examples:
  gt config agent set claude-glm \"claude-glm --model glm-4\"
  gt config agent set gemini-custom gemini --approval-mode yolo
  gt config agent set claude \"claude-glm\"  # Override built-in claude`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigAgentSet,
}

var configAgentRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove custom agent",
	Long: `Remove a custom agent definition from town settings.

This removes a custom agent from your town settings. Built-in agents
(claude, gemini, codex) cannot be removed.

Examples:
  gt config agent remove claude-glm`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAgentRemove,
}

// Cost-tier subcommand

var configCostTierCmd = &cobra.Command{
	Use:   "cost-tier [tier]",
	Short: "Get or set cost optimization tier",
	Long: `Get or set the cost optimization tier for model selection.

With no arguments, shows the current cost tier and role assignments.
With an argument, applies the specified tier preset.

Tiers control which AI model each role uses:
  standard  All roles use Opus (highest quality, default)
  economy   Patrol roles use Sonnet/Haiku, workers use Opus
  budget    Patrol roles use Haiku, workers use Sonnet

Examples:
  gt config cost-tier              # Show current tier
  gt config cost-tier economy      # Switch to economy tier
  gt config cost-tier standard     # Reset to all-Opus`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConfigCostTier,
}

func runConfigCostTier(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if len(args) == 0 {
		// Show current tier and role assignments
		current := config.GetCurrentTier(townSettings)
		if current == "" {
			fmt.Println("Cost tier: " + style.Bold.Render("custom") + " (manual role_agents configuration)")
		} else {
			tier := config.CostTier(current)
			fmt.Printf("Cost tier: %s\n", style.Bold.Render(current))
			fmt.Printf("  %s\n\n", config.TierDescription(tier))
			fmt.Println("Role assignments:")
			fmt.Println(config.FormatTierRoleTable(tier))
		}
		return nil
	}

	// Apply tier
	tierName := args[0]
	if !config.IsValidTier(tierName) {
		return fmt.Errorf("invalid cost tier %q (valid: %s)", tierName, strings.Join(config.ValidCostTiers(), ", "))
	}

	tier := config.CostTier(tierName)

	// Warn if overwriting custom role_agents
	currentTier := config.GetCurrentTier(townSettings)
	if currentTier == "" && len(townSettings.RoleAgents) > 0 {
		fmt.Println("Warning: overwriting custom role_agents configuration")
	}

	if err := config.ApplyCostTier(townSettings, tier); err != nil {
		return fmt.Errorf("applying cost tier: %w", err)
	}

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Cost tier set to %s\n", style.Bold.Render(tierName))
	fmt.Printf("  %s\n\n", config.TierDescription(tier))
	fmt.Println("Role assignments:")
	fmt.Println(config.FormatTierRoleTable(tier))
	return nil
}

// Default-agent subcommand

var configDefaultAgentCmd = &cobra.Command{
	Use:   "default-agent [name]",
	Short: "Get or set default agent",
	Long: `Get or set the default agent for the town.

With no arguments, shows the current default agent.
With an argument, sets the default agent to the specified name.

The default agent is used when a rig doesn't specify its own agent
setting. Can be a built-in preset (claude, gemini, codex) or a
custom agent name.

Examples:
  gt config default-agent           # Show current default
  gt config default-agent claude    # Set to claude
  gt config default-agent gemini    # Set to gemini
  gt config default-agent my-custom # Set to custom agent`,
	RunE: runConfigDefaultAgent,
}

var configAgentEmailDomainCmd = &cobra.Command{
	Use:   "agent-email-domain [domain]",
	Short: "Get or set agent email domain",
	Long: `Get or set the domain used for agent git commit emails.

When agents commit code via 'gt commit', their identity is converted
to a git email address. For example, "gastown/crew/jack" becomes
"gastown.crew.jack@{domain}".

With no arguments, shows the current domain.
With an argument, sets the domain.

Default: gastown.local

Examples:
  gt config agent-email-domain                 # Show current domain
  gt config agent-email-domain gastown.local   # Set to gastown.local
  gt config agent-email-domain example.com     # Set custom domain`,
	RunE: runConfigAgentEmailDomain,
}

// Flags
var (
	configAgentListJSON bool
)

// AgentListItem represents an agent in list output.
type AgentListItem struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Args     string `json:"args,omitempty"`
	Type     string `json:"type"` // "built-in" or "custom"
	IsCustom bool   `json:"is_custom"`
}

func runConfigAgentList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load town settings
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	// Load agent registry
	registryPath := config.DefaultAgentRegistryPath(townRoot)
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		return fmt.Errorf("loading agent registry: %w", err)
	}

	// Collect all agents
	builtInAgents := config.ListAgentPresets()
	customAgents := make(map[string]*config.RuntimeConfig)
	if townSettings.Agents != nil {
		for name, runtime := range townSettings.Agents {
			customAgents[name] = runtime
		}
	}

	// Build list items
	var items []AgentListItem
	for _, name := range builtInAgents {
		preset := config.GetAgentPresetByName(name)
		if preset != nil {
			items = append(items, AgentListItem{
				Name:     name,
				Command:  preset.Command,
				Args:     strings.Join(preset.Args, " "),
				Type:     "built-in",
				IsCustom: false,
			})
		}
	}
	for name, runtime := range customAgents {
		argsStr := ""
		if runtime.Args != nil {
			argsStr = strings.Join(runtime.Args, " ")
		}
		items = append(items, AgentListItem{
			Name:     name,
			Command:  runtime.Command,
			Args:     argsStr,
			Type:     "custom",
			IsCustom: true,
		})
	}

	// Sort by name
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	if configAgentListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Text output
	fmt.Printf("%s\n\n", style.Bold.Render("Available Agents"))
	for _, item := range items {
		typeLabel := style.Dim.Render("[" + item.Type + "]")
		fmt.Printf("  %s %s %s", style.Bold.Render(item.Name), typeLabel, item.Command)
		if item.Args != "" {
			fmt.Printf(" %s", item.Args)
		}
		fmt.Println()
	}

	// Show default
	defaultAgent := townSettings.DefaultAgent
	if defaultAgent == "" {
		defaultAgent = "claude"
	}
	fmt.Printf("\nDefault: %s\n", style.Bold.Render(defaultAgent))

	return nil
}

func runConfigAgentGet(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load town settings for custom agents
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	// Load agent registry
	registryPath := config.DefaultAgentRegistryPath(townRoot)
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		return fmt.Errorf("loading agent registry: %w", err)
	}

	// Check custom agents first
	if townSettings.Agents != nil {
		if runtime, ok := townSettings.Agents[name]; ok {
			displayAgentConfig(name, runtime, nil, true)
			return nil
		}
	}

	// Check built-in agents
	preset := config.GetAgentPresetByName(name)
	if preset != nil {
		runtime := &config.RuntimeConfig{
			Command: preset.Command,
			Args:    preset.Args,
		}
		displayAgentConfig(name, runtime, preset, false)
		return nil
	}

	return fmt.Errorf("agent '%s' not found", name)
}

func displayAgentConfig(name string, runtime *config.RuntimeConfig, preset *config.AgentPresetInfo, isCustom bool) {
	fmt.Printf("%s\n\n", style.Bold.Render("Agent: "+name))

	typeLabel := "custom"
	if !isCustom {
		typeLabel = "built-in"
	}
	fmt.Printf("Type:   %s\n", typeLabel)
	fmt.Printf("Command: %s\n", runtime.Command)

	if runtime.Args != nil && len(runtime.Args) > 0 {
		fmt.Printf("Args:    %s\n", strings.Join(runtime.Args, " "))
	}

	if preset != nil {
		if preset.SessionIDEnv != "" {
			fmt.Printf("Session ID Env: %s\n", preset.SessionIDEnv)
		}
		if preset.ResumeFlag != "" {
			fmt.Printf("Resume Style:  %s (%s)\n", preset.ResumeStyle, preset.ResumeFlag)
		}
		fmt.Printf("Supports Hooks: %v\n", preset.SupportsHooks)
	}
}

func runConfigAgentSet(cmd *cobra.Command, args []string) error {
	name := args[0]
	commandLine := args[1]

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load town settings
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	// Parse command line into command and args
	parts := strings.Fields(commandLine)
	if len(parts) == 0 {
		return fmt.Errorf("command cannot be empty")
	}

	// Initialize agents map if needed
	if townSettings.Agents == nil {
		townSettings.Agents = make(map[string]*config.RuntimeConfig)
	}

	// Create or update the agent
	townSettings.Agents[name] = &config.RuntimeConfig{
		Command: parts[0],
		Args:    parts[1:],
	}

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Agent '%s' set to: %s\n", style.Bold.Render(name), commandLine)

	// Check if this overrides a built-in
	builtInAgents := config.ListAgentPresets()
	for _, builtin := range builtInAgents {
		if name == builtin {
			fmt.Printf("\n%s\n", style.Dim.Render("(overriding built-in '"+builtin+"' preset)"))
			break
		}
	}

	return nil
}

func runConfigAgentRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Check if trying to remove built-in
	builtInAgents := config.ListAgentPresets()
	for _, builtin := range builtInAgents {
		if name == builtin {
			return fmt.Errorf("cannot remove built-in agent '%s' (use 'gt config agent set' to override it)", name)
		}
	}

	// Load town settings
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	// Check if agent exists
	if townSettings.Agents == nil || townSettings.Agents[name] == nil {
		return fmt.Errorf("custom agent '%s' not found", name)
	}

	// Remove the agent
	delete(townSettings.Agents, name)

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Removed custom agent '%s'\n", style.Bold.Render(name))
	return nil
}

func runConfigDefaultAgent(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load town settings
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	// Load agent registry
	registryPath := config.DefaultAgentRegistryPath(townRoot)
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		return fmt.Errorf("loading agent registry: %w", err)
	}

	if len(args) == 0 {
		// Show current default
		defaultAgent := townSettings.DefaultAgent
		if defaultAgent == "" {
			defaultAgent = "claude"
		}
		fmt.Printf("Default agent: %s\n", style.Bold.Render(defaultAgent))
		return nil
	}

	// Set new default
	name := args[0]

	// Verify agent exists
	isValid := false
	builtInAgents := config.ListAgentPresets()
	for _, builtin := range builtInAgents {
		if name == builtin {
			isValid = true
			break
		}
	}
	if !isValid && townSettings.Agents != nil {
		if _, ok := townSettings.Agents[name]; ok {
			isValid = true
		}
	}

	if !isValid {
		return fmt.Errorf("agent '%s' not found (use 'gt config agent list' to see available agents)", name)
	}

	// Set default
	townSettings.DefaultAgent = name

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Default agent set to '%s'\n", style.Bold.Render(name))
	return nil
}

func runConfigAgentEmailDomain(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load town settings
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if len(args) == 0 {
		// Show current domain
		domain := townSettings.AgentEmailDomain
		if domain == "" {
			domain = DefaultAgentEmailDomain
		}
		fmt.Printf("Agent email domain: %s\n", style.Bold.Render(domain))
		fmt.Printf("\nExample: gastown/crew/jack → gastown.crew.jack@%s\n", domain)
		return nil
	}

	// Set new domain
	domain := args[0]

	// Basic validation - domain should not be empty and should not start with @
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if strings.HasPrefix(domain, "@") {
		return fmt.Errorf("domain should not include @: use '%s' instead", strings.TrimPrefix(domain, "@"))
	}

	// Set domain
	townSettings.AgentEmailDomain = domain

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Agent email domain set to '%s'\n", style.Bold.Render(domain))
	fmt.Printf("\nExample: gastown/crew/jack → gastown.crew.jack@%s\n", domain)
	return nil
}

// configSetCmd sets a town config value by dot-notation key.
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a town configuration value using dot-notation keys.

Supported keys:
  convoy.notify_on_complete   Push notification to Mayor session on convoy
                              completion (true/false, default: false)
  cli_theme                   CLI color scheme ("dark", "light", "auto")
  default_agent               Default agent preset name
  scheduler.max_polecats      Dispatch mode: -1 = direct (default), N > 0 = deferred
  scheduler.batch_size        Beads per heartbeat (default: 1)
  scheduler.spawn_delay       Delay between spawns (default: 0s)

Examples:
  gt config set convoy.notify_on_complete true
  gt config set cli_theme dark
  gt config set default_agent claude
  gt config set scheduler.max_polecats 5
  gt config set scheduler.max_polecats -1`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

// configGetCmd gets a town config value by dot-notation key.
var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a town configuration value using dot-notation keys.

Supported keys:
  convoy.notify_on_complete   Push notification to Mayor session on convoy
                              completion (true/false, default: false)
  cli_theme                   CLI color scheme
  default_agent               Default agent preset name
  scheduler.max_polecats      Dispatch mode (-1 = direct, N > 0 = deferred)
  scheduler.batch_size        Beads per heartbeat
  scheduler.spawn_delay       Delay between spawns

Examples:
  gt config get convoy.notify_on_complete
  gt config get cli_theme
  gt config get scheduler.max_polecats`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	switch key {
	case "convoy.notify_on_complete":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w (expected true/false)", key, err)
		}
		if townSettings.Convoy == nil {
			townSettings.Convoy = &config.ConvoyConfig{}
		}
		townSettings.Convoy.NotifyOnComplete = b

	case "cli_theme":
		switch value {
		case "dark", "light", "auto":
			townSettings.CLITheme = value
		default:
			return fmt.Errorf("invalid cli_theme: %q (expected dark, light, or auto)", value)
		}

	case "default_agent":
		townSettings.DefaultAgent = value

	case "scheduler.max_polecats":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w (expected integer)", key, err)
		}
		if n < -1 {
			return fmt.Errorf("invalid value for %s: must be >= -1 (-1 = direct dispatch, 0 = direct dispatch, N > 0 = deferred)", key)
		}
		if townSettings.Scheduler == nil {
			townSettings.Scheduler = capacity.DefaultSchedulerConfig()
		}
		townSettings.Scheduler.MaxPolecats = &n

	case "scheduler.batch_size":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return fmt.Errorf("invalid value for %s: expected positive integer", key)
		}
		if townSettings.Scheduler == nil {
			townSettings.Scheduler = capacity.DefaultSchedulerConfig()
		}
		townSettings.Scheduler.BatchSize = &n

	case "scheduler.spawn_delay":
		// Validate it parses as a duration
		_, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w (expected Go duration, e.g. 2s, 500ms)", key, err)
		}
		if townSettings.Scheduler == nil {
			townSettings.Scheduler = capacity.DefaultSchedulerConfig()
		}
		townSettings.Scheduler.SpawnDelay = value

	default:
		return fmt.Errorf("unknown config key: %q\n\nSupported keys:\n  convoy.notify_on_complete\n  cli_theme\n  default_agent\n  scheduler.max_polecats\n  scheduler.batch_size\n  scheduler.spawn_delay", key)
	}

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Set %s = %s\n", style.Bold.Render(key), value)
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	var value string
	switch key {
	case "convoy.notify_on_complete":
		if townSettings.Convoy != nil && townSettings.Convoy.NotifyOnComplete {
			value = "true"
		} else {
			value = "false"
		}

	case "cli_theme":
		value = townSettings.CLITheme
		if value == "" {
			value = "auto"
		}

	case "default_agent":
		value = townSettings.DefaultAgent
		if value == "" {
			value = "claude"
		}

	case "scheduler.max_polecats":
		scfg := townSettings.Scheduler
		if scfg == nil {
			scfg = capacity.DefaultSchedulerConfig()
		}
		value = strconv.Itoa(scfg.GetMaxPolecats())

	case "scheduler.batch_size":
		scfg := townSettings.Scheduler
		if scfg == nil {
			scfg = capacity.DefaultSchedulerConfig()
		}
		value = strconv.Itoa(scfg.GetBatchSize())

	case "scheduler.spawn_delay":
		scfg := townSettings.Scheduler
		if scfg == nil {
			scfg = capacity.DefaultSchedulerConfig()
		}
		value = scfg.GetSpawnDelay().String()

	default:
		return fmt.Errorf("unknown config key: %q\n\nSupported keys:\n  convoy.notify_on_complete\n  cli_theme\n  default_agent\n  scheduler.max_polecats\n  scheduler.batch_size\n  scheduler.spawn_delay", key)
	}

	fmt.Println(value)
	return nil
}

// parseBool parses a boolean string (true/false, yes/no, 1/0).
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "yes", "1", "on":
		return true, nil
	case "false", "no", "0", "off":
		return false, nil
	default:
		return false, fmt.Errorf("cannot parse %q as boolean", s)
	}
}

func init() {
	// Add flags
	configAgentListCmd.Flags().BoolVar(&configAgentListJSON, "json", false, "Output as JSON")

	// Add agent subcommands
	configAgentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent configuration",
		Long: `Manage per-agent configuration settings.

Subcommands allow listing, getting, setting, and removing agent-specific
config values such as the default AI model or provider.`,
		RunE: requireSubcommand,
	}
	configAgentCmd.AddCommand(configAgentListCmd)
	configAgentCmd.AddCommand(configAgentGetCmd)
	configAgentCmd.AddCommand(configAgentSetCmd)
	configAgentCmd.AddCommand(configAgentRemoveCmd)

	// Add subcommands to config
	configCmd.AddCommand(configAgentCmd)
	configCmd.AddCommand(configCostTierCmd)
	configCmd.AddCommand(configDefaultAgentCmd)
	configCmd.AddCommand(configAgentEmailDomainCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)

	// Register with root
	rootCmd.AddCommand(configCmd)
}
