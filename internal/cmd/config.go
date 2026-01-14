// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wisp"
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

// Seance subcommands

var configSeanceCmd = &cobra.Command{
	Use:   "seance",
	Short: "Manage auto-seance configuration",
	Long: `Manage auto-seance context recovery settings.

Auto-seance automatically recovers context from predecessor sessions
when starting work on "cold" projects (no recent activity).

Commands:
  gt config seance            Show current settings
  gt config seance enable     Enable auto-seance
  gt config seance disable    Disable auto-seance
  gt config seance threshold  Get or set cold threshold`,
	RunE: runConfigSeanceShow,
}

var configSeanceEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable auto-seance",
	RunE:  runConfigSeanceEnable,
}

var configSeanceDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable auto-seance",
	RunE:  runConfigSeanceDisable,
}

var configSeanceThresholdCmd = &cobra.Command{
	Use:   "threshold [duration]",
	Short: "Get or set cold threshold",
	Long: `Get or set the cold threshold for auto-seance.

The cold threshold determines how long a rig must be inactive before
auto-seance runs on startup. Default is 24h.

Duration format: 1h, 30m, 2h30m, 24h, 48h, etc.

Examples:
  gt config seance threshold         # Show current threshold
  gt config seance threshold 12h     # Set to 12 hours
  gt config seance threshold 48h     # Set to 48 hours`,
	RunE: runConfigSeanceThreshold,
}

func runConfigSeanceShow(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Detect rig from cwd
	rigName := detectRigFromCwd(townRoot)
	if rigName == "" {
		rigName = "default"
	}

	wispCfg := wisp.NewConfig(townRoot, rigName)

	// Get enabled status
	enabled := true // default
	if val := wispCfg.Get("seance.enabled"); val != nil {
		switch v := val.(type) {
		case bool:
			enabled = v
		case string:
			enabled = v == "true" || v == "1" || v == "yes"
		}
	}

	// Get threshold
	threshold := "24h" // default
	if val := wispCfg.Get("seance.cold_threshold"); val != nil {
		if s, ok := val.(string); ok {
			threshold = s
		}
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Auto-Seance Configuration"))
	fmt.Printf("Rig:       %s\n", rigName)
	fmt.Printf("Enabled:   %v\n", enabled)
	fmt.Printf("Threshold: %s\n", threshold)
	fmt.Printf("\nConfig file: %s\n", style.Dim.Render(wispCfg.ConfigPath()))
	return nil
}

func runConfigSeanceEnable(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	rigName := detectRigFromCwd(townRoot)
	if rigName == "" {
		rigName = "default"
	}

	wispCfg := wisp.NewConfig(townRoot, rigName)
	if err := wispCfg.Set("seance.enabled", true); err != nil {
		return fmt.Errorf("setting config: %w", err)
	}

	fmt.Printf("Auto-seance enabled for rig '%s'\n", style.Bold.Render(rigName))
	return nil
}

func runConfigSeanceDisable(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	rigName := detectRigFromCwd(townRoot)
	if rigName == "" {
		rigName = "default"
	}

	wispCfg := wisp.NewConfig(townRoot, rigName)
	if err := wispCfg.Set("seance.enabled", false); err != nil {
		return fmt.Errorf("setting config: %w", err)
	}

	fmt.Printf("Auto-seance disabled for rig '%s'\n", style.Bold.Render(rigName))
	return nil
}

func runConfigSeanceThreshold(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	rigName := detectRigFromCwd(townRoot)
	if rigName == "" {
		rigName = "default"
	}

	wispCfg := wisp.NewConfig(townRoot, rigName)

	if len(args) == 0 {
		// Show current threshold
		threshold := "24h" // default
		if val := wispCfg.Get("seance.cold_threshold"); val != nil {
			if s, ok := val.(string); ok {
				threshold = s
			}
		}
		fmt.Printf("Cold threshold: %s\n", style.Bold.Render(threshold))
		return nil
	}

	// Set new threshold - validate duration format
	thresholdStr := args[0]
	if _, err := time.ParseDuration(thresholdStr); err != nil {
		return fmt.Errorf("invalid duration '%s': use format like 1h, 30m, 24h, 48h", thresholdStr)
	}

	if err := wispCfg.Set("seance.cold_threshold", thresholdStr); err != nil {
		return fmt.Errorf("setting config: %w", err)
	}

	fmt.Printf("Cold threshold set to '%s' for rig '%s'\n", style.Bold.Render(thresholdStr), rigName)
	return nil
}

// detectRigFromCwd detects the rig name from current working directory.
func detectRigFromCwd(townRoot string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	relPath, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return ""
	}

	// First path component after town root is the rig name
	// (unless it's mayor, deacon, or other top-level dirs)
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) == 0 || parts[0] == "." || parts[0] == "" {
		return ""
	}

	// Skip non-rig directories
	first := parts[0]
	if first == "mayor" || first == "deacon" || first == ".beads" || first == "settings" {
		return ""
	}

	return first
}

func init() {
	// Add flags
	configAgentListCmd.Flags().BoolVar(&configAgentListJSON, "json", false, "Output as JSON")

	// Add agent subcommands
	configAgentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent configuration",
		RunE:  requireSubcommand,
	}
	configAgentCmd.AddCommand(configAgentListCmd)
	configAgentCmd.AddCommand(configAgentGetCmd)
	configAgentCmd.AddCommand(configAgentSetCmd)
	configAgentCmd.AddCommand(configAgentRemoveCmd)

	// Add seance subcommands
	configSeanceCmd.AddCommand(configSeanceEnableCmd)
	configSeanceCmd.AddCommand(configSeanceDisableCmd)
	configSeanceCmd.AddCommand(configSeanceThresholdCmd)

	// Add subcommands to config
	configCmd.AddCommand(configAgentCmd)
	configCmd.AddCommand(configDefaultAgentCmd)
	configCmd.AddCommand(configAgentEmailDomainCmd)
	configCmd.AddCommand(configSeanceCmd)

	// Register with root
	rootCmd.AddCommand(configCmd)
}
