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
  gt config agent <name>             Set default agent (shorthand)
  gt config agent list               List all agents (built-in and custom)
  gt config agent get <name>         Show agent configuration
  gt config agent set <name> <cmd>   Set custom agent command
  gt config agent remove <name>      Remove custom agent
  gt config add-agent <name>         Add custom agent with full configuration
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

var configAddAgentCmd = &cobra.Command{
	Use:   "add-agent <name>",
	Short: "Add a custom agent with full configuration",
	Long: `Add a custom agent with comprehensive configuration options.

This command allows you to define a new agent with all configuration options
including hooks, session management, and process detection settings.

The agent will be saved to settings/agents.json and become available for
use with role assignments and as the default agent.

Examples:
  # Add a basic custom agent
  gt config add-agent kiro --command kiro-cli

  # Add an agent with hooks support
  gt config add-agent kiro --command kiro-cli --hooks-provider kiro --supports-hooks

  # Add an agent with full configuration
  gt config add-agent my-agent \
    --command my-agent-cli \
    --args "--autonomous,--no-confirm" \
    --process-names "my-agent,my-agent-cli" \
    --session-id-env MY_AGENT_SESSION_ID \
    --resume-flag "--resume" \
    --resume-style flag \
    --supports-hooks \
    --hooks-provider my-agent \
    --hooks-dir ".my-agent" \
    --hooks-settings-file "settings.json"`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAddAgent,
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

// validRoles defines the valid role names for per-role agent configuration.
var validRoles = []string{"mayor", "deacon", "witness", "refinery", "polecat", "crew"}

var configRoleAgentCmd = &cobra.Command{
	Use:   "role-agent <role> [agent]",
	Short: "Get or set agent for a specific role",
	Long: `Get or set the agent used for a specific role.

With one argument (role), shows the current agent for that role.
With two arguments (role, agent), sets the agent for that role.

This allows cost optimization by assigning different agents or models
to different roles. For example, use a cheaper model for witness
(which does monitoring) and a more capable model for polecats
(which do complex coding tasks).

Valid roles: mayor, deacon, witness, refinery, polecat, crew

The agent can include a model suffix using colon syntax:
  <agent>:<model>  e.g., "claude:haiku" or "claude:opus"

When model syntax is used, a custom agent entry is created combining
the base agent's settings with the model name as a suffix.

Examples:
  gt config role-agent witness                  # Show witness agent
  gt config role-agent deacon opencode          # Set deacon to opencode
  gt config role-agent witness claude:haiku     # Set witness to claude:haiku
  gt config role-agent polecat claude:opus      # Set polecat to claude:opus`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runConfigRoleAgent,
}

// Flags
var (
	configAgentListJSON bool

	// add-agent flags
	addAgentCommand          string
	addAgentArgs             string
	addAgentProcessNames     string
	addAgentSessionIDEnv     string
	addAgentResumeFlag       string
	addAgentResumeStyle      string
	addAgentSupportsHooks    bool
	addAgentHooksProvider    string
	addAgentHooksDir         string
	addAgentHooksSettingsFile string
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

func runConfigAddAgent(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate required flags
	if addAgentCommand == "" {
		return fmt.Errorf("--command is required")
	}

	// Validate resume style if provided
	if addAgentResumeStyle != "" && addAgentResumeStyle != "flag" && addAgentResumeStyle != "subcommand" {
		return fmt.Errorf("--resume-style must be 'flag' or 'subcommand'")
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Build the AgentPresetInfo
	agentInfo := &config.AgentPresetInfo{
		Name:          config.AgentPreset(name),
		Command:       addAgentCommand,
		SessionIDEnv:  addAgentSessionIDEnv,
		ResumeFlag:    addAgentResumeFlag,
		ResumeStyle:   addAgentResumeStyle,
		SupportsHooks: addAgentSupportsHooks,
	}

	// Parse comma-separated args
	if addAgentArgs != "" {
		agentInfo.Args = strings.Split(addAgentArgs, ",")
	}

	// Parse comma-separated process names
	if addAgentProcessNames != "" {
		agentInfo.ProcessNames = strings.Split(addAgentProcessNames, ",")
	}

	// Load or create agent registry
	registryPath := config.DefaultAgentRegistryPath(townRoot)
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		return fmt.Errorf("loading agent registry: %w", err)
	}

	// Check if agent already exists (warn if overwriting)
	existingPreset := config.GetAgentPresetByName(name)
	isOverwrite := existingPreset != nil

	// Build the registry entry
	registry := &config.AgentRegistry{
		Version: config.CurrentAgentRegistryVersion,
		Agents:  make(map[string]*config.AgentPresetInfo),
	}

	// Load existing custom agents from file if it exists
	existingData, err := os.ReadFile(registryPath)
	if err == nil {
		if jsonErr := json.Unmarshal(existingData, registry); jsonErr != nil {
			// File exists but is invalid, start fresh
			registry.Agents = make(map[string]*config.AgentPresetInfo)
		}
	}

	// Add the new agent
	registry.Agents[name] = agentInfo

	// Save to registry file
	if err := config.SaveAgentRegistry(registryPath, registry); err != nil {
		return fmt.Errorf("saving agent registry: %w", err)
	}

	// Also add to town settings for RuntimeConfig compatibility
	settingsPath := config.TownSettingsPath(townRoot)
	townSettings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading town settings: %w", err)
	}

	if townSettings.Agents == nil {
		townSettings.Agents = make(map[string]*config.RuntimeConfig)
	}

	// Create RuntimeConfig from AgentPresetInfo
	runtimeConfig := &config.RuntimeConfig{
		Command: agentInfo.Command,
		Args:    agentInfo.Args,
	}

	// Set hooks config if provided
	if addAgentHooksProvider != "" || addAgentHooksDir != "" || addAgentHooksSettingsFile != "" {
		runtimeConfig.Hooks = &config.RuntimeHooksConfig{
			Provider:     addAgentHooksProvider,
			Dir:          addAgentHooksDir,
			SettingsFile: addAgentHooksSettingsFile,
		}
	}

	townSettings.Agents[name] = runtimeConfig

	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	// Output result
	if isOverwrite {
		fmt.Printf("Updated agent '%s'\n", style.Bold.Render(name))
	} else {
		fmt.Printf("Added agent '%s'\n", style.Bold.Render(name))
	}

	fmt.Printf("  Command: %s\n", agentInfo.Command)
	if len(agentInfo.Args) > 0 {
		fmt.Printf("  Args: %s\n", strings.Join(agentInfo.Args, " "))
	}
	if len(agentInfo.ProcessNames) > 0 {
		fmt.Printf("  Process names: %s\n", strings.Join(agentInfo.ProcessNames, ", "))
	}
	if agentInfo.SessionIDEnv != "" {
		fmt.Printf("  Session ID env: %s\n", agentInfo.SessionIDEnv)
	}
	if agentInfo.ResumeFlag != "" {
		fmt.Printf("  Resume: %s (%s)\n", agentInfo.ResumeFlag, agentInfo.ResumeStyle)
	}
	if agentInfo.SupportsHooks {
		fmt.Printf("  Hooks: enabled\n")
		if addAgentHooksProvider != "" {
			fmt.Printf("    Provider: %s\n", addAgentHooksProvider)
		}
		if addAgentHooksDir != "" {
			fmt.Printf("    Dir: %s\n", addAgentHooksDir)
		}
	}

	fmt.Printf("\nSaved to: %s\n", style.Dim.Render(registryPath))

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

func runConfigRoleAgent(cmd *cobra.Command, args []string) error {
	role := args[0]

	// Validate role
	isValidRole := false
	for _, r := range validRoles {
		if role == r {
			isValidRole = true
			break
		}
	}
	if !isValidRole {
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

	// Load agent registry
	registryPath := config.DefaultAgentRegistryPath(townRoot)
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		return fmt.Errorf("loading agent registry: %w", err)
	}

	if len(args) == 1 {
		// Show current agent for role
		agentName := ""
		if townSettings.RoleAgents != nil {
			agentName = townSettings.RoleAgents[role]
		}
		if agentName == "" {
			defaultAgent := townSettings.DefaultAgent
			if defaultAgent == "" {
				defaultAgent = "claude"
			}
			fmt.Printf("Role %s: %s %s\n", style.Bold.Render(role), defaultAgent, style.Dim.Render("(default)"))
		} else {
			fmt.Printf("Role %s: %s\n", style.Bold.Render(role), style.Bold.Render(agentName))
		}
		return nil
	}

	// Set agent for role
	agentArg := args[1]

	// Parse agent:model syntax
	agentName := agentArg
	modelSuffix := ""
	if colonIdx := strings.Index(agentArg, ":"); colonIdx > 0 {
		agentName = agentArg[:colonIdx]
		modelSuffix = agentArg[colonIdx+1:]
	}

	// Verify base agent exists
	isValid := false
	builtInAgents := config.ListAgentPresets()
	for _, builtin := range builtInAgents {
		if agentName == builtin {
			isValid = true
			break
		}
	}
	if !isValid && townSettings.Agents != nil {
		if _, ok := townSettings.Agents[agentName]; ok {
			isValid = true
		}
	}

	if !isValid {
		return fmt.Errorf("agent '%s' not found (use 'gt config agent list' to see available agents)", agentName)
	}

	// If model suffix is provided, create a custom agent entry
	finalAgentName := agentArg
	if modelSuffix != "" {
		// Create custom agent name like "claude-haiku"
		customAgentName := agentName + "-" + modelSuffix

		// Initialize agents map if needed
		if townSettings.Agents == nil {
			townSettings.Agents = make(map[string]*config.RuntimeConfig)
		}

		// Only create if it doesn't already exist
		if _, exists := townSettings.Agents[customAgentName]; !exists {
			// Get base agent config
			basePreset := config.GetAgentPresetByName(agentName)
			if basePreset != nil {
				// Create new config with model in args
				newArgs := append([]string{}, basePreset.Args...)
				// Add model argument (agent-specific handling)
				switch agentName {
				case "claude":
					newArgs = append(newArgs, "--model", modelSuffix)
				case "gemini":
					newArgs = append(newArgs, "--model", modelSuffix)
				default:
					// Generic: just append as model arg
					newArgs = append(newArgs, "--model", modelSuffix)
				}
				townSettings.Agents[customAgentName] = &config.RuntimeConfig{
					Command: basePreset.Command,
					Args:    newArgs,
				}
				fmt.Printf("Created custom agent '%s' with model '%s'\n", style.Bold.Render(customAgentName), modelSuffix)
			}
		}
		finalAgentName = customAgentName
	}

	// Initialize RoleAgents map if needed
	if townSettings.RoleAgents == nil {
		townSettings.RoleAgents = make(map[string]string)
	}

	// Set role agent
	townSettings.RoleAgents[role] = finalAgentName

	// Save settings
	if err := config.SaveTownSettings(settingsPath, townSettings); err != nil {
		return fmt.Errorf("saving town settings: %w", err)
	}

	fmt.Printf("Role '%s' set to agent '%s'\n", style.Bold.Render(role), style.Bold.Render(finalAgentName))
	return nil
}

// runConfigAgent handles the 'gt config agent' command.
// When called with an argument, sets the default agent.
// When called without arguments, shows help.
func runConfigAgent(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	// With an argument, treat as setting the default agent
	// This provides the shorthand: gt config agent claude
	return runConfigDefaultAgent(cmd, args)
}

func init() {
	// Add flags
	configAgentListCmd.Flags().BoolVar(&configAgentListJSON, "json", false, "Output as JSON")

	// Add add-agent flags
	configAddAgentCmd.Flags().StringVar(&addAgentCommand, "command", "", "CLI command to invoke (required)")
	configAddAgentCmd.Flags().StringVar(&addAgentArgs, "args", "", "Default arguments (comma-separated)")
	configAddAgentCmd.Flags().StringVar(&addAgentProcessNames, "process-names", "", "Process names for detection (comma-separated)")
	configAddAgentCmd.Flags().StringVar(&addAgentSessionIDEnv, "session-id-env", "", "Environment variable for session ID")
	configAddAgentCmd.Flags().StringVar(&addAgentResumeFlag, "resume-flag", "", "Flag or subcommand for resuming sessions")
	configAddAgentCmd.Flags().StringVar(&addAgentResumeStyle, "resume-style", "", "Resume style: 'flag' or 'subcommand'")
	configAddAgentCmd.Flags().BoolVar(&addAgentSupportsHooks, "supports-hooks", false, "Whether agent supports hooks system")
	configAddAgentCmd.Flags().StringVar(&addAgentHooksProvider, "hooks-provider", "", "Hooks provider name")
	configAddAgentCmd.Flags().StringVar(&addAgentHooksDir, "hooks-dir", "", "Hooks directory (e.g., '.kiro')")
	configAddAgentCmd.Flags().StringVar(&addAgentHooksSettingsFile, "hooks-settings-file", "", "Hooks settings file name")
	_ = configAddAgentCmd.MarkFlagRequired("command")

	// Add agent subcommands
	configAgentCmd := &cobra.Command{
		Use:   "agent [name]",
		Short: "Manage agent configuration or set default agent",
		Long: `Manage agent configuration or set the default agent.

When called with an agent name, sets that agent as the default:
  gt config agent claude      Set claude as default agent
  gt config agent gemini      Set gemini as default agent

Subcommands for agent management:
  gt config agent list        List all agents (built-in and custom)
  gt config agent get <name>  Show agent configuration
  gt config agent set <name> <cmd>   Set custom agent command
  gt config agent remove <name>      Remove custom agent`,
		RunE: runConfigAgent,
	}
	configAgentCmd.AddCommand(configAgentListCmd)
	configAgentCmd.AddCommand(configAgentGetCmd)
	configAgentCmd.AddCommand(configAgentSetCmd)
	configAgentCmd.AddCommand(configAgentRemoveCmd)

	// Add subcommands to config
	configCmd.AddCommand(configAgentCmd)
	configCmd.AddCommand(configAddAgentCmd)
	configCmd.AddCommand(configDefaultAgentCmd)
	configCmd.AddCommand(configAgentEmailDomainCmd)
	configCmd.AddCommand(configRoleAgentCmd)

	// Register with root
	rootCmd.AddCommand(configCmd)
}
