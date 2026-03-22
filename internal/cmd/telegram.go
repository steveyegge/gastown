package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	telegram "github.com/steveyegge/gastown/internal/bridge/telegram"
	"github.com/steveyegge/gastown/internal/workspace"
)

var telegramCmd = &cobra.Command{
	Use:     "telegram",
	GroupID: GroupServices,
	Short:   "Telegram bridge for overseer communication",
	Long:    "Manage the Telegram bridge that lets you chat with the Mayor and receive notifications over Telegram.",
	RunE:    requireSubcommand,
}

func init() {
	rootCmd.AddCommand(telegramCmd)
	telegramCmd.AddCommand(newTelegramConfigureCmd())
	telegramCmd.AddCommand(newTelegramStatusCmd())
	telegramCmd.AddCommand(newTelegramRunCmd())
}

// --- configure ---

type telegramConfigureFlags struct {
	token     string
	chatID    int64
	allowFrom []int64
	notify    []string
	yes       bool
}

func newTelegramConfigureCmd() *cobra.Command {
	f := &telegramConfigureFlags{}

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure the Telegram bridge",
		Long: `Configure the Telegram bridge with a bot token, chat ID, and access controls.

Loads the existing config (if any), applies the provided flags, validates, and saves.

Examples:
  gt telegram configure --token=123456:ABC... --chat-id=987654321
  gt telegram configure --allow-from=111222333 --notify=escalations,errors
  gt telegram configure --token=<new-token> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTelegramConfigure(cmd, f)
		},
	}

	cmd.Flags().StringVar(&f.token, "token", "", "Telegram bot token (from @BotFather)")
	cmd.Flags().Int64Var(&f.chatID, "chat-id", 0, "Telegram chat ID to send messages to")
	cmd.Flags().Int64SliceVar(&f.allowFrom, "allow-from", nil, "Allowed sender user IDs (comma-separated)")
	cmd.Flags().StringSliceVar(&f.notify, "notify", nil, "Notification categories to enable (comma-separated)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "Skip confirmation prompts")

	return cmd
}

func runTelegramConfigure(cmd *cobra.Command, f *telegramConfigureFlags) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	configPath := telegram.ConfigPath(townRoot)

	// Load existing config if present.
	var cfg telegram.Config
	existing, loadErr := telegram.LoadConfig(configPath)
	if loadErr == nil {
		cfg = existing
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		// File exists but couldn't be loaded (bad perms, parse error, etc.)
		return fmt.Errorf("loading existing config: %w", loadErr)
	}

	// Apply only the flags that were explicitly set.
	if cmd.Flags().Changed("token") {
		if cfg.Token != "" && cfg.Token != f.token && !f.yes {
			fmt.Printf("Replacing existing token (%s).\n", cfg.MaskedToken())
			fmt.Print("Continue? [y/N] ")
			var answer string
			fmt.Scanln(&answer) //nolint:errcheck
			if answer != "y" && answer != "Y" {
				fmt.Println("Aborted.")
				return nil
			}
		}
		cfg.Token = f.token
	}
	if cmd.Flags().Changed("chat-id") {
		cfg.ChatID = f.chatID
	}
	if cmd.Flags().Changed("allow-from") {
		cfg.AllowFrom = f.allowFrom
	}
	if cmd.Flags().Changed("notify") {
		cfg.Notify = f.notify
	}

	cfg.Enabled = true
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if err := telegram.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Telegram bridge configured (%s).\n", configPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  gt telegram status    # verify configuration")
	fmt.Println("  gt telegram run       # start the bridge")
	return nil
}

// --- status ---

type telegramStatusFlags struct {
	jsonOutput bool
}

func newTelegramStatusCmd() *cobra.Command {
	f := &telegramStatusFlags{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Telegram bridge configuration status",
		Long: `Show the current Telegram bridge configuration.

The bot token is masked for security. Use --json for machine-readable output.

Examples:
  gt telegram status
  gt telegram status --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTelegramStatus(f)
		},
	}

	cmd.Flags().BoolVar(&f.jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runTelegramStatus(f *telegramStatusFlags) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	configPath := telegram.ConfigPath(townRoot)
	cfg, err := telegram.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Telegram bridge: not configured")
			fmt.Println()
			fmt.Println("Run 'gt telegram configure --help' to get started.")
			return nil
		}
		return fmt.Errorf("loading config: %w", err)
	}

	if f.jsonOutput {
		// Mask token before JSON output.
		out := struct {
			Token     string   `json:"token"`
			ChatID    int64    `json:"chat_id"`
			AllowFrom []int64  `json:"allow_from,omitempty"`
			Target    string   `json:"target,omitempty"`
			Enabled   bool     `json:"enabled"`
			Notify    []string `json:"notify,omitempty"`
			RateLimit int      `json:"rate_limit,omitempty"`
		}{
			Token:     cfg.MaskedToken(),
			ChatID:    cfg.ChatID,
			AllowFrom: cfg.AllowFrom,
			Target:    cfg.Target,
			Enabled:   cfg.Enabled,
			Notify:    cfg.Notify,
			RateLimit: cfg.RateLimit,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output.
	enabledStr := "no"
	if cfg.IsEnabled() {
		enabledStr = "yes"
	}
	fmt.Printf("Telegram bridge status\n")
	fmt.Printf("  Config:    %s\n", configPath)
	fmt.Printf("  Enabled:   %s\n", enabledStr)
	fmt.Printf("  Token:     %s\n", cfg.MaskedToken())
	fmt.Printf("  Chat ID:   %d\n", cfg.ChatID)
	fmt.Printf("  Target:    %s\n", cfg.Target)
	if len(cfg.AllowFrom) > 0 {
		fmt.Printf("  Allow from: %v\n", cfg.AllowFrom)
	} else {
		fmt.Printf("  Allow from: (none — all users blocked)\n")
	}
	if len(cfg.Notify) > 0 {
		fmt.Printf("  Notify:    %v\n", cfg.Notify)
	}
	if cfg.RateLimit > 0 {
		fmt.Printf("  Rate limit: %d msg/min\n", cfg.RateLimit)
	}
	return nil
}

// --- run ---

func newTelegramRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the Telegram bridge in the foreground",
		Long: `Start the Telegram bridge and run it in the foreground.

The bridge polls Telegram for incoming messages and relays them to the Mayor.
It also watches the feed for outbound notifications and delivers them to your
Telegram chat.

Press Ctrl-C to stop.

Examples:
  gt telegram run`,
		RunE: runTelegramRun,
	}
}

func runTelegramRun(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	configPath := telegram.ConfigPath(townRoot)
	cfg, err := telegram.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("telegram bridge is not configured — run 'gt telegram configure' first")
		}
		return fmt.Errorf("loading config: %w", err)
	}

	if !cfg.IsEnabled() {
		return fmt.Errorf("telegram bridge is disabled — set enabled=true in %s", configPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel context on SIGTERM or SIGINT for clean shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		signal.Reset(syscall.SIGTERM, syscall.SIGINT) // restore default so second signal kills immediately
		fmt.Println("\nShutting down Telegram bridge...")
		cancel()
	}()

	sender := telegram.NewCLISender(townRoot)
	bridge := telegram.NewBridge(cfg, sender, townRoot)

	fmt.Printf("Starting Telegram bridge (token: %s, chat: %d)...\n", cfg.MaskedToken(), cfg.ChatID)

	if err := bridge.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("bridge exited: %w", err)
	}
	return nil
}
