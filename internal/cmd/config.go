// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
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

// Role-agents subcommands

var configRoleAgentsCmd = &cobra.Command{
	Use:   "role-agents",
	Short: "Manage per-role agent configuration",
	Long: `Manage which agent (model) is used for each role.

This allows cost optimization by using cheaper models for simpler roles.
For example, use Haiku for witness/refinery and Sonnet for mayor/crew.

Commands:
  gt config role-agents              List all role→agent mappings
  gt config role-agents set <role> <agent>   Set agent for a role
  gt config role-agents remove <role>        Remove role override

Valid roles: mayor, deacon, witness, refinery, polecat, crew

Examples:
  gt config role-agents                      # Show current mappings
  gt config role-agents set witness claude-haiku
  gt config role-agents set polecat claude
  gt config role-agents remove witness`,
	RunE: runConfigRoleAgentsList,
}

var configRoleAgentsSetCmd = &cobra.Command{
	Use:   "set <role> <agent>",
	Short: "Set agent for a role",
	Long: `Set which agent to use for a specific role.

This creates or updates a role→agent mapping in town settings.
The agent must be a valid built-in preset or custom agent.

Valid roles: mayor, deacon, witness, refinery, polecat, crew

Examples:
  gt config role-agents set witness claude-haiku
  gt config role-agents set polecat gemini
  gt config role-agents set deacon claude-haiku`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigRoleAgentsSet,
}

var configRoleAgentsRemoveCmd = &cobra.Command{
	Use:   "remove <role>",
	Short: "Remove role agent override",
	Long: `Remove the agent override for a role.

After removal, the role will use the default agent resolution
(rig agent → town default_agent → built-in default).

Examples:
  gt config role-agents remove witness
  gt config role-agents remove polecat`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigRoleAgentsRemove,
}

// Flags
var (
	configAgentListJSON     bool
	configRoleAgentsListJSON bool
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

// Valid role names for role-agents configuration.
var validRoles = []string{"mayor", "deacon", "witness", "refinery", "polecat", "crew"}

func isValidRole(role string) bool {
	for _, r := range validRoles {
		if role == r {
			return true
		}
	}
	return false
}

// RoleAgentItem represents a role→agent mapping in list output.
type RoleAgentItem struct {
	Role   string `json:"role"`
	Agent  string `json:"agent"`
	Source string `json:"source"` // "configured" or "default"
}

func runConfigRoleAgentsList(cmd *cobra.Command, args []string) error {
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

	// Build list of role→agent mappings
	defaultAgent := townSettings.DefaultAgent
	if defaultAgent == "" {
		defaultAgent = "claude"
	}

	var items []RoleAgentItem
	for _, role := range validRoles {
		agent := defaultAgent
		source := "default"
		if townSettings.RoleAgents != nil {
			if configured, ok := townSettings.RoleAgents[role]; ok && configured != "" {
				agent = configured
				source = "configured"
			}
		}
		items = append(items, RoleAgentItem{
			Role:   role,
			Agent:  agent,
			Source: source,
		})
	}

	if configRoleAgentsListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Text output
	fmt.Printf("%s\n\n", style.Bold.Render("Role Agent Mappings"))
	for _, item := range items {
		sourceLabel := ""
		if item.Source == "default" {
			sourceLabel = style.Dim.Render(" (default)")
		}
		fmt.Printf("  %-10s → %s%s\n", item.Role, style.Bold.Render(item.Agent), sourceLabel)
	}

	fmt.Printf("\nDefault agent: %s\n", style.Bold.Render(defaultAgent))
	fmt.Printf("\nUse 'gt config role-agents set <role> <agent>' to override.\n")
	fmt.Printf("See 'gt config agent list' for available agents.\n")

	return nil
}

func runConfigRoleAgentsSet(cmd *cobra.Command, args []string) error {
	role := args[0]
	agent := args[1]

	// Validate role
	if !isValidRole(role) {
		return fmt.Errorf("invalid role '%s' (valid roles: %s)", role, strings.Join(validRoles, ", "))
	}

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

	// Load agent registry to validate agent exists
	registryPath := config.DefaultAgentRegistryPath(townRoot)
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		return fmt.Errorf("loading agent registry: %w", err)
	}

	// Verify agent exists
	isValid := false
	builtInAgents := config.ListAgentPresets()
	for _, builtin := range builtInAgents {
		if agent == builtin {
			isValid = true
			break
		}
	}
	if !isValid && townSettings.Agents != nil {
		if _, ok := townSettings.Agents[agent]; ok {
			isValid = true
		}
	}

	if !isValid {
		return fmt.Errorf("agent '%s' not found (use 'gt config agent list' to see available agents)", agent)
	}

	// Initialize role_agents map if needed
	if townSettings.RoleAgents == nil {
		townSettings.RoleAgents = make(map[string]string)
	}

	// Set the role→agent mapping
	oldAgent := townSettings.RoleAgents[role]
	townSettings.RoleAgents[role] = agent

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	if oldAgent != "" && oldAgent != agent {
		fmt.Printf("Role '%s' changed from '%s' to '%s'\n", style.Bold.Render(role), oldAgent, style.Bold.Render(agent))
	} else {
		fmt.Printf("Role '%s' set to agent '%s'\n", style.Bold.Render(role), style.Bold.Render(agent))
	}

	return nil
}

func runConfigRoleAgentsRemove(cmd *cobra.Command, args []string) error {
	role := args[0]

	// Validate role
	if !isValidRole(role) {
		return fmt.Errorf("invalid role '%s' (valid roles: %s)", role, strings.Join(validRoles, ", "))
	}

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

	// Check if role has an override
	if townSettings.RoleAgents == nil || townSettings.RoleAgents[role] == "" {
		return fmt.Errorf("role '%s' has no agent override configured", role)
	}

	// Remove the override
	oldAgent := townSettings.RoleAgents[role]
	delete(townSettings.RoleAgents, role)

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	defaultAgent := townSettings.DefaultAgent
	if defaultAgent == "" {
		defaultAgent = "claude"
	}

	fmt.Printf("Removed agent override for role '%s' (was '%s')\n", style.Bold.Render(role), oldAgent)
	fmt.Printf("Role '%s' will now use default agent '%s'\n", role, style.Bold.Render(defaultAgent))

	return nil
}

func init() {
	// Add flags
	configAgentListCmd.Flags().BoolVar(&configAgentListJSON, "json", false, "Output as JSON")
	configRoleAgentsCmd.Flags().BoolVar(&configRoleAgentsListJSON, "json", false, "Output as JSON")

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

	// Add role-agents subcommands
	configRoleAgentsCmd.AddCommand(configRoleAgentsSetCmd)
	configRoleAgentsCmd.AddCommand(configRoleAgentsRemoveCmd)

	// Add subcommands to config
	configCmd.AddCommand(configAgentCmd)
	configCmd.AddCommand(configDefaultAgentCmd)
	configCmd.AddCommand(configAgentEmailDomainCmd)
	configCmd.AddCommand(configRoleAgentsCmd)

	// Register with root
	rootCmd.AddCommand(configCmd)
}
