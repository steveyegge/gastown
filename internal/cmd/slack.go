package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/slack"
	"github.com/steveyegge/gastown/internal/slackbot"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
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

// Slack bot start command flags
var (
	slackBotToken        string
	slackAppToken        string
	slackRPCURL          string
	slackChannelID       string
	slackDynamicChannels bool
	slackChannelPrefix   string
	slackTownRoot        string
	slackAutoInvite      string
	slackDebug           bool
	slackHealthPort      int
	slackEventSource     string // "auto", "bus", "sse"
	slackNatsURL         string // NATS server URL
	slackNatsToken       string // NATS auth token
)

const slackLockFile = "/tmp/gtslack.lock"

var slackStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Slack bot",
	Long: `Start the Gas Town Slack bot for decision management.

The bot connects to Slack via Socket Mode and to the RPC server to
allow humans to view and resolve pending decisions from Slack.

Event sources (--event-source):
  auto - Try bd bus (NATS) first, fall back to SSE (default)
  bus  - Use bd bus (NATS JetStream) only
  sse  - Use SSE endpoint only (legacy)

Environment variables can also be used:
  SLACK_BOT_TOKEN   - Bot OAuth token (xoxb-...)
  SLACK_APP_TOKEN   - App-level token for Socket Mode (xapp-...)
  GTMOBILE_RPC      - RPC endpoint URL
  SLACK_CHANNEL     - Channel ID for decision notifications
  SLACK_AUTO_INVITE - Comma-separated Slack user IDs to auto-invite
  HEALTH_PORT       - HTTP health endpoint port (default: 8080)
  BD_NATS_PORT      - NATS port (default: 4222)
  BD_DAEMON_TOKEN   - NATS auth token

Health endpoints for Kubernetes probes:
  /healthz - Liveness probe (checks Slack connection)
  /readyz  - Readiness probe (always ready once started)

Examples:
  gt slack start -bot-token=xoxb-... -app-token=xapp-...
  gt slack start --channel=C12345 --dynamic-channels
  gt slack start --event-source=bus --nats-url=nats://localhost:4222`,
	RunE: runSlackStart,
}

// Channel mode commands
var slackModeCmd = &cobra.Command{
	Use:   "channel-mode",
	Short: "Manage agent channel mode preferences",
	Long: `Manage channel routing mode preferences for agents.

Channel modes determine how an agent's decisions are routed:
  general - Route to the default/general channel
  agent   - Route to a dedicated per-agent channel
  epic    - Route to a channel based on the work's parent epic
  dm      - Route as a direct message to the overseer

Examples:
  gt slack channel-mode                            # Show default mode
  gt slack channel-mode get gastown/crew/joe       # Get agent's mode
  gt slack channel-mode set gastown/crew/joe epic  # Set agent's mode
  gt slack channel-mode clear gastown/crew/joe     # Clear agent's mode`,
	RunE: runSlackModeShow,
}

var slackModeGetCmd = &cobra.Command{
	Use:   "get <agent>",
	Short: "Get channel mode for agent",
	Long:  `Get the channel routing mode preference for a specific agent.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runSlackModeGet,
}

var slackModeSetCmd = &cobra.Command{
	Use:   "set <agent> <mode>",
	Short: "Set channel mode for agent",
	Long: `Set the channel routing mode preference for a specific agent.

Valid modes: general, agent, epic, dm`,
	Args: cobra.ExactArgs(2),
	RunE: runSlackModeSet,
}

var slackModeClearCmd = &cobra.Command{
	Use:   "clear <agent>",
	Short: "Clear channel mode for agent",
	Long:  `Clear the channel routing mode preference for a specific agent. The agent will use the default mode.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runSlackModeClear,
}

var slackModeDefaultCmd = &cobra.Command{
	Use:   "default [mode]",
	Short: "Get or set the default channel mode",
	Long: `Get or set the default channel routing mode for all agents.

If no mode is provided, shows the current default.
If a mode is provided, sets it as the new default.

Valid modes: general, agent, epic, dm`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSlackModeDefault,
}

// Slack post command for agents to post to decision threads (gt-8d5q52.2)
var slackPostDecisionID string

var slackPostCmd = &cobra.Command{
	Use:   "post <message>",
	Short: "Post a message to a decision thread",
	Long: `Post a non-decision message to an existing decision thread.

This allows agents to provide additional context, analysis, or follow-up
information in a decision thread without creating a new decision.

Requires the RPC server to be running.

Examples:
  gt slack post --decision gt-abc123 "Here's my analysis of the options..."
  gt slack post -d gt-xyz789 "I've investigated further and found..."`,
	Args: cobra.ExactArgs(1),
	RunE: runSlackPost,
}

func init() {
	rootCmd.AddCommand(slackCmd)

	slackCmd.AddCommand(slackStatusCmd)
	slackCmd.AddCommand(slackRouteCmd)
	slackCmd.AddCommand(slackMigrateCmd)
	slackCmd.AddCommand(slackModeCmd)
	slackCmd.AddCommand(slackStartCmd)
	slackCmd.AddCommand(slackPostCmd)

	slackRouteCmd.AddCommand(slackRouteListCmd)
	slackRouteCmd.AddCommand(slackRouteSetCmd)
	slackRouteCmd.AddCommand(slackRouteRemoveCmd)

	slackModeCmd.AddCommand(slackModeGetCmd)
	slackModeCmd.AddCommand(slackModeSetCmd)
	slackModeCmd.AddCommand(slackModeClearCmd)
	slackModeCmd.AddCommand(slackModeDefaultCmd)

	slackRouteListCmd.Flags().BoolVar(&slackRouteJSON, "json", false, "Output as JSON")
	slackRouteSetCmd.Flags().StringVar(&slackRouteChannelName, "name", "", "Human-readable channel name")

	// Slack post command flags
	slackPostCmd.Flags().StringVarP(&slackPostDecisionID, "decision", "d", "", "Decision ID to post to (required)")
	_ = slackPostCmd.MarkFlagRequired("decision")

	// Slack bot start command flags
	slackStartCmd.Flags().StringVar(&slackBotToken, "bot-token", "", "Slack bot token (xoxb-...)")
	slackStartCmd.Flags().StringVar(&slackAppToken, "app-token", "", "Slack app token for Socket Mode (xapp-...)")
	slackStartCmd.Flags().StringVar(&slackRPCURL, "rpc", "http://localhost:8443", "RPC endpoint URL")
	slackStartCmd.Flags().StringVar(&slackChannelID, "channel", "", "Default channel ID for decision notifications")
	slackStartCmd.Flags().BoolVar(&slackDynamicChannels, "dynamic-channels", false, "Enable automatic channel creation per agent")
	slackStartCmd.Flags().StringVar(&slackChannelPrefix, "channel-prefix", "gt-decisions", "Prefix for dynamically created channels")
	slackStartCmd.Flags().StringVar(&slackTownRoot, "town-root", "", "Town root directory (auto-discovered if empty)")
	slackStartCmd.Flags().StringVar(&slackAutoInvite, "auto-invite", "", "Comma-separated Slack user IDs to auto-invite")
	slackStartCmd.Flags().BoolVar(&slackDebug, "debug", false, "Enable debug logging")
	slackStartCmd.Flags().IntVar(&slackHealthPort, "health-port", 8080, "HTTP health endpoint port for K8s probes")
	slackStartCmd.Flags().StringVar(&slackEventSource, "event-source", "auto", "Event source: auto (bus with SSE fallback), bus (NATS only), sse (SSE only)")
	slackStartCmd.Flags().StringVar(&slackNatsURL, "nats-url", "nats://localhost:4222", "NATS server URL for bus event subscription")
	slackStartCmd.Flags().StringVar(&slackNatsToken, "nats-token", "", "NATS auth token (default: BD_DAEMON_TOKEN env)")
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

func runSlackModeShow(cmd *cobra.Command, args []string) error {
	// Show default mode
	mode, err := slack.GetDefaultChannelMode()
	if err != nil {
		return fmt.Errorf("get default mode: %w", err)
	}

	if mode == "" {
		fmt.Println("Default channel mode: (not set, using general)")
	} else {
		fmt.Printf("Default channel mode: %s\n", mode)
	}

	fmt.Println()
	fmt.Println("Valid modes: general, agent, epic, dm")
	fmt.Println()
	fmt.Println("Use 'gt slack channel-mode get <agent>' to check an agent's mode")
	fmt.Println("Use 'gt slack channel-mode set <agent> <mode>' to set an agent's mode")

	return nil
}

func runSlackModeGet(cmd *cobra.Command, args []string) error {
	agent := args[0]

	mode, err := slack.GetAgentChannelMode(agent)
	if err != nil {
		return fmt.Errorf("get mode for %s: %w", agent, err)
	}

	if mode == "" {
		// Check default
		defaultMode, _ := slack.GetDefaultChannelMode()
		if defaultMode == "" {
			defaultMode = "general"
		}
		fmt.Printf("%s: (not set, using default: %s)\n", agent, defaultMode)
	} else {
		fmt.Printf("%s: %s\n", agent, mode)
	}

	return nil
}

func runSlackModeSet(cmd *cobra.Command, args []string) error {
	agent := args[0]
	mode := args[1]

	if !slack.IsValidChannelMode(mode) {
		return fmt.Errorf("invalid mode %q: must be one of general, agent, epic, dm", mode)
	}

	if err := slack.SetAgentChannelMode(agent, slack.ChannelMode(mode)); err != nil {
		return fmt.Errorf("set mode for %s: %w", agent, err)
	}

	printSuccess("Set channel mode for %s: %s", agent, mode)
	return nil
}

func runSlackModeClear(cmd *cobra.Command, args []string) error {
	agent := args[0]

	if err := slack.ClearAgentChannelMode(agent); err != nil {
		return fmt.Errorf("clear mode for %s: %w", agent, err)
	}

	printSuccess("Cleared channel mode for %s (will use default)", agent)
	return nil
}

func runSlackModeDefault(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// Get default
		mode, err := slack.GetDefaultChannelMode()
		if err != nil {
			return fmt.Errorf("get default mode: %w", err)
		}

		if mode == "" {
			fmt.Println("Default channel mode: (not set, using general)")
		} else {
			fmt.Printf("Default channel mode: %s\n", mode)
		}
		return nil
	}

	// Set default
	mode := args[0]
	if !slack.IsValidChannelMode(mode) {
		return fmt.Errorf("invalid mode %q: must be one of general, agent, epic, dm", mode)
	}

	if err := slack.SetDefaultChannelMode(slack.ChannelMode(mode)); err != nil {
		return fmt.Errorf("set default mode: %w", err)
	}

	printSuccess("Set default channel mode: %s", mode)
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

func runSlackStart(cmd *cobra.Command, args []string) error {
	// Check if this should be managed by systemd instead
	if os.Getenv("GT_SLACK_SYSTEMD") == "" {
		// Not launched by systemd - check if systemd unit exists and is enabled
		if isSystemdUnitEnabled("gt-slack.service") {
			fmt.Fprintln(os.Stderr, style.Warning.Render("⚠ gt slack start is managed by systemd"))
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Use systemctl to manage the Slack bot:")
			fmt.Fprintln(os.Stderr, "  systemctl --user status gt-slack    # Check status")
			fmt.Fprintln(os.Stderr, "  systemctl --user restart gt-slack   # Restart after rebuild")
			fmt.Fprintln(os.Stderr, "  journalctl --user -u gt-slack -f    # View logs")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "To disable systemd management:")
			fmt.Fprintln(os.Stderr, "  systemctl --user disable gt-slack")
			return fmt.Errorf("use 'systemctl --user restart gt-slack' instead")
		}
	}

	// Acquire exclusive lock to prevent multiple instances.
	// This prevents duplicate Slack notifications from concurrent processes.
	fileLock := flock.New(slackLockFile)
	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another slack bot instance is already running (lock file: %s)", slackLockFile)
	}
	defer func() { _ = fileLock.Unlock() }()

	// Allow environment variable overrides
	if slackBotToken == "" {
		slackBotToken = os.Getenv("SLACK_BOT_TOKEN")
	}
	if slackAppToken == "" {
		slackAppToken = os.Getenv("SLACK_APP_TOKEN")
	}
	if os.Getenv("GTMOBILE_RPC") != "" {
		slackRPCURL = os.Getenv("GTMOBILE_RPC")
	}
	if slackChannelID == "" {
		slackChannelID = os.Getenv("SLACK_CHANNEL")
	}
	if slackAutoInvite == "" {
		slackAutoInvite = os.Getenv("SLACK_AUTO_INVITE")
	}

	// Allow HEALTH_PORT env var to override flag default
	if envPort := os.Getenv("HEALTH_PORT"); envPort != "" && slackHealthPort == 8080 {
		if port, err := strconv.Atoi(envPort); err == nil {
			slackHealthPort = port
		}
	}
	if slackNatsToken == "" {
		slackNatsToken = os.Getenv("BD_DAEMON_TOKEN")
	}
	if natsPort := os.Getenv("BD_NATS_PORT"); natsPort != "" && slackNatsURL == "nats://localhost:4222" {
		slackNatsURL = "nats://localhost:" + natsPort
	}

	if slackBotToken == "" || slackAppToken == "" {
		return fmt.Errorf("both --bot-token and --app-token are required (or set SLACK_BOT_TOKEN and SLACK_APP_TOKEN)")
	}

	// Parse auto-invite user IDs (comma-separated)
	var autoInviteUsers []string
	if slackAutoInvite != "" {
		for _, u := range strings.Split(slackAutoInvite, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				autoInviteUsers = append(autoInviteUsers, u)
			}
		}
	}

	// Auto-discover town root if not specified
	townRoot := slackTownRoot
	if townRoot == "" {
		townRoot, _ = workspace.FindFromCwd()
	}

	cfg := slackbot.Config{
		BotToken:        slackBotToken,
		AppToken:        slackAppToken,
		RPCEndpoint:     slackRPCURL,
		ChannelID:       slackChannelID,
		DynamicChannels: slackDynamicChannels,
		ChannelPrefix:   slackChannelPrefix,
		TownRoot:        townRoot,
		AutoInviteUsers: autoInviteUsers,
		Debug:           slackDebug,
	}

	bot, err := slackbot.New(cfg)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	log.Printf("Starting Gas Town Slack bot")
	log.Printf("RPC endpoint: %s", slackRPCURL)
	if slackChannelID != "" {
		log.Printf("Default notifications channel: %s", slackChannelID)
	}
	if slackDynamicChannels {
		log.Printf("Dynamic channel creation enabled (prefix: %s)", slackChannelPrefix)
	}
	if len(autoInviteUsers) > 0 {
		log.Printf("Auto-invite users: %v", autoInviteUsers)
	}

	// Start health server for Kubernetes probes
	if slackHealthPort > 0 {
		healthServer := slackbot.NewHealthServer(bot, slackHealthPort)
		go func() {
			if err := healthServer.Start(ctx); err != nil && ctx.Err() == nil {
				log.Printf("Health server error: %v", err)
			}
		}()
	}

	// Start event listener for real-time decision notifications
	if slackChannelID != "" {
		eventSource := slackEventSource
		if eventSource == "" {
			eventSource = "auto"
		}

		switch eventSource {
		case "bus":
			startBusListener(ctx, bot, false)
		case "sse":
			startSSEListener(ctx, bot)
		default: // "auto"
			startBusListener(ctx, bot, true)
		}
	}

	if err := bot.Run(ctx); err != nil {
		// Graceful shutdown via SIGTERM/SIGINT results in context.Canceled
		// This is expected behavior, not an error - return nil for clean exit
		if ctx.Err() == context.Canceled {
			log.Println("Slack bot stopped gracefully")
			return nil
		}
		return fmt.Errorf("bot error: %w", err)
	}
	return nil
}

// isSystemdUnitEnabled checks if a systemd user unit is enabled.
// startBusListener starts the NATS-based bus event listener.
// If fallbackToSSE is true, falls back to SSE on NATS connection failure.
func startBusListener(ctx context.Context, bot *slackbot.Bot, fallbackToSSE bool) {
	busCfg := slackbot.BusListenerConfig{
		NatsURL:   slackNatsURL,
		AuthToken: slackNatsToken,
	}
	busListener := slackbot.NewBusListener(busCfg, bot, bot.RPCClient())

	go func() {
		log.Printf("Starting Bus listener (NATS): %s", slackNatsURL)
		if err := busListener.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Bus listener error: %v", err)
			if fallbackToSSE {
				log.Printf("Falling back to SSE listener")
				startSSEListener(ctx, bot)
			}
		}
	}()
}

// startSSEListener starts the SSE-based event listener (legacy).
func startSSEListener(ctx context.Context, bot *slackbot.Bot) {
	sseURL := slackRPCURL + "/events/decisions"
	sseListener := slackbot.NewSSEListener(sseURL, bot, bot.RPCClient())
	go func() {
		log.Printf("Starting SSE listener: %s", sseURL)
		if err := sseListener.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("SSE listener error: %v", err)
		}
	}()
}

func isSystemdUnitEnabled(unit string) bool {
	cmd := exec.Command("systemctl", "--user", "is-enabled", unit)
	err := cmd.Run()
	return err == nil
}

// runSlackPost posts a message to a decision thread (gt-8d5q52.2).
// This allows agents to post non-decision responses in decision threads.
func runSlackPost(cmd *cobra.Command, args []string) error {
	message := args[0]
	decisionID := slackPostDecisionID

	if decisionID == "" {
		return fmt.Errorf("--decision flag is required")
	}

	// For now, print a helpful message - the full implementation requires
	// the slackbot to listen for thread_post events via SSE
	fmt.Println("Thread post feature is being implemented (gt-8d5q52.2).")
	fmt.Printf("Decision: %s\n", decisionID)
	fmt.Printf("Message: %s\n", message)
	fmt.Println()
	fmt.Println("For now, use 'gt decision request --predecessor' to post follow-up decisions.")
	fmt.Println("Non-decision thread posts will be available in a future release.")

	return nil
}
