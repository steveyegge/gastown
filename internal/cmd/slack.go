package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/slack"
	"github.com/steveyegge/gastown/internal/style"
)

// printSuccess prints a success message with a checkmark prefix.
func printSuccess(format string, args ...interface{}) {
	fmt.Printf("✓ "+format+"\n", args...)
}

var slackCmd = &cobra.Command{
	Use:     "slack",
	GroupID: GroupConfig,
	Short:   "Manage Slack channel routing",
	Long: `Manage Slack channel routing for decision notifications.

Channel routing determines which Slack channel receives decision notifications
for each agent. Configuration can be stored in beads (recommended) or a JSON file.

Examples:
  gt slack status              # Show current routing config
  gt slack route list          # List all routing rules
  gt slack route set <agent> <channel>   # Add/update agent channel
  gt slack migrate             # Migrate file config to beads`,
}

var slackStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Slack routing status",
	Long:  `Display the current Slack routing configuration and its source (beads or file).`,
	RunE:  runSlackStatus,
}

var slackRouteCmd = &cobra.Command{
	Use:   "route",
	Short: "Manage channel routing rules",
	Long:  `Manage agent-to-channel routing rules for Slack notifications.`,
}

var slackRouteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List routing rules",
	Long:  `List all channel routing rules including patterns and overrides.`,
	RunE:  runSlackRouteList,
}

var slackRouteSetCmd = &cobra.Command{
	Use:   "set <agent> <channel-id>",
	Short: "Set channel for agent",
	Long: `Set the Slack channel for a specific agent or pattern.

Examples:
  gt slack route set gastown/crew/slack_decisions C0ABD8BUDTR
  gt slack route set "gastown/polecats/*" C0POLECATS123`,
	Args: cobra.ExactArgs(2),
	RunE: runSlackRouteSet,
}

var slackRouteRemoveCmd = &cobra.Command{
	Use:   "remove <agent>",
	Short: "Remove channel override for agent",
	Long:  `Remove a channel override for a specific agent.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runSlackRouteRemove,
}

var slackMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate config to beads",
	Long: `Migrate Slack routing configuration from file to beads.

This reads the current file-based configuration and stores it in beads config.
After migration, the router will use beads as the primary config source.`,
	RunE: runSlackMigrate,
}

var slackRouteJSON bool
var slackRouteChannelName string

func init() {
	rootCmd.AddCommand(slackCmd)

	slackCmd.AddCommand(slackStatusCmd)
	slackCmd.AddCommand(slackRouteCmd)
	slackCmd.AddCommand(slackMigrateCmd)

	slackRouteCmd.AddCommand(slackRouteListCmd)
	slackRouteCmd.AddCommand(slackRouteSetCmd)
	slackRouteCmd.AddCommand(slackRouteRemoveCmd)

	slackRouteListCmd.Flags().BoolVar(&slackRouteJSON, "json", false, "Output as JSON")
	slackRouteSetCmd.Flags().StringVar(&slackRouteChannelName, "name", "", "Human-readable channel name")
}

func runSlackStatus(cmd *cobra.Command, args []string) error {
	router, err := slack.LoadRouter()
	if err != nil {
		return fmt.Errorf("load router: %w", err)
	}

	cfg := router.GetConfig()

	fmt.Println("Slack Channel Routing Status")
	fmt.Println("============================")
	fmt.Println()

	// Source
	if router.IsBeadsBacked() {
		printSuccess("Config source: beads (bd config slack.*)")
	} else {
		fmt.Println("Config source: file (settings/slack.json)")
	}
	fmt.Println()

	// Status
	if cfg.Enabled {
		printSuccess("Status: enabled")
	} else {
		style.PrintWarning("Status: disabled")
	}

	// Default channel
	fmt.Printf("Default channel: %s", cfg.DefaultChannel)
	if name, ok := cfg.ChannelNames[cfg.DefaultChannel]; ok {
		fmt.Printf(" (%s)", name)
	}
	fmt.Println()

	// Stats
	fmt.Printf("Pattern rules: %d\n", len(cfg.Channels))
	fmt.Printf("Agent overrides: %d\n", len(cfg.Overrides))

	return nil
}

func runSlackRouteList(cmd *cobra.Command, args []string) error {
	router, err := slack.LoadRouter()
	if err != nil {
		return fmt.Errorf("load router: %w", err)
	}

	cfg := router.GetConfig()

	if slackRouteJSON {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Print patterns
	if len(cfg.Channels) > 0 {
		fmt.Println("Pattern Rules:")
		for pattern, channel := range cfg.Channels {
			name := cfg.ChannelNames[channel]
			if name != "" {
				fmt.Printf("  %s → %s (%s)\n", pattern, channel, name)
			} else {
				fmt.Printf("  %s → %s\n", pattern, channel)
			}
		}
		fmt.Println()
	}

	// Print overrides
	if len(cfg.Overrides) > 0 {
		fmt.Println("Agent Overrides:")
		for agent, channel := range cfg.Overrides {
			name := cfg.ChannelNames[channel]
			if name != "" {
				fmt.Printf("  %s → %s (%s)\n", agent, channel, name)
			} else {
				fmt.Printf("  %s → %s\n", agent, channel)
			}
		}
		fmt.Println()
	}

	// Print default
	fmt.Printf("Default: %s", cfg.DefaultChannel)
	if name, ok := cfg.ChannelNames[cfg.DefaultChannel]; ok {
		fmt.Printf(" (%s)", name)
	}
	fmt.Println()

	return nil
}

func runSlackRouteSet(cmd *cobra.Command, args []string) error {
	agent := args[0]
	channelID := args[1]

	router, err := slack.LoadRouter()
	if err != nil {
		return fmt.Errorf("load router: %w", err)
	}

	// Add the override
	if slackRouteChannelName != "" {
		router.AddOverrideWithName(agent, channelID, slackRouteChannelName)
	} else {
		router.AddOverride(agent, channelID)
	}

	// Save
	if err := router.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	printSuccess("Set %s → %s", agent, channelID)
	if slackRouteChannelName != "" {
		fmt.Printf("Channel name: %s\n", slackRouteChannelName)
	}

	return nil
}

func runSlackRouteRemove(cmd *cobra.Command, args []string) error {
	agent := args[0]

	router, err := slack.LoadRouter()
	if err != nil {
		return fmt.Errorf("load router: %w", err)
	}

	prev := router.RemoveOverride(agent)
	if prev == "" {
		style.PrintWarning("No override found for %s", agent)
		return nil
	}

	// Save
	if err := router.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	printSuccess("Removed override for %s (was %s)", agent, prev)
	return nil
}

func runSlackMigrate(cmd *cobra.Command, args []string) error {
	router, err := slack.LoadRouter()
	if err != nil {
		return fmt.Errorf("load router: %w", err)
	}

	if router.IsBeadsBacked() {
		style.PrintWarning("Config is already backed by beads")
		return nil
	}

	// Show what will be migrated
	cfg := router.GetConfig()
	fmt.Println("Migrating Slack config to beads:")
	fmt.Printf("  Enabled: %v\n", cfg.Enabled)
	fmt.Printf("  Default channel: %s\n", cfg.DefaultChannel)
	fmt.Printf("  Patterns: %d\n", len(cfg.Channels))
	fmt.Printf("  Overrides: %d\n", len(cfg.Overrides))
	fmt.Println()

	// Migrate
	if err := router.MigrateToBeads(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	printSuccess("Migrated Slack config to beads")
	fmt.Println()
	fmt.Println("The file-based config is no longer used.")
	fmt.Println("You can safely remove settings/slack.json if desired.")
	fmt.Println()
	fmt.Println("To verify: gt slack status")

	// Sync beads
	fmt.Println()
	fmt.Println("Syncing beads...")
	syncCmd := exec.Command("bd", "sync")
	syncCmd.Stdout = os.Stdout
	syncCmd.Stderr = os.Stderr
	_ = syncCmd.Run()

	return nil
}
