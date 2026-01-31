// Package main implements the Gas Town Slack bot for decision management.
//
// The bot connects to Slack via Socket Mode and to gtmobile via RPC to
// allow humans to view and resolve pending decisions from Slack.
//
// Usage:
//
//	gtslack -bot-token=xoxb-... -app-token=xapp-... [-rpc=http://localhost:8443] [-channel=C12345]
//
// Environment variables can also be used:
//
//	SLACK_BOT_TOKEN   - Bot OAuth token (xoxb-...)
//	SLACK_APP_TOKEN   - App-level token for Socket Mode (xapp-...)
//	GTMOBILE_RPC      - gtmobile RPC endpoint URL
//	SLACK_CHANNEL     - Channel ID for decision notifications
//	SLACK_AUTO_INVITE - Comma-separated Slack user IDs to auto-invite when routing to channels
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/slackbot"
)

const lockFile = "/tmp/gtslack.lock"

var (
	botToken        = flag.String("bot-token", "", "Slack bot token (xoxb-...)")
	appToken        = flag.String("app-token", "", "Slack app token for Socket Mode (xapp-...)")
	rpcURL          = flag.String("rpc", "http://localhost:8443", "gtmobile RPC endpoint URL")
	channelID       = flag.String("channel", "", "Default channel ID for decision notifications")
	dynamicChannels = flag.Bool("dynamic-channels", false, "Enable automatic channel creation per agent")
	channelPrefix   = flag.String("channel-prefix", "gt-decisions", "Prefix for dynamically created channels")
	townRoot        = flag.String("town-root", "", "Town root directory for convoy lookup (auto-discovered if empty)")
	autoInvite      = flag.String("auto-invite", "", "Comma-separated Slack user IDs to auto-invite when routing to channels")
	debug           = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	// Acquire exclusive lock to prevent multiple instances.
	// This prevents duplicate Slack notifications from concurrent processes.
	fileLock := flock.New(lockFile)
	locked, err := fileLock.TryLock()
	if err != nil {
		log.Fatalf("Failed to acquire lock: %v", err)
	}
	if !locked {
		log.Fatalf("Another gtslack instance is already running (lock file: %s)", lockFile)
	}
	defer func() { _ = fileLock.Unlock() }()

	// Allow environment variable overrides
	if *botToken == "" {
		*botToken = os.Getenv("SLACK_BOT_TOKEN")
	}
	if *appToken == "" {
		*appToken = os.Getenv("SLACK_APP_TOKEN")
	}
	if os.Getenv("GTMOBILE_RPC") != "" {
		*rpcURL = os.Getenv("GTMOBILE_RPC")
	}
	if *channelID == "" {
		*channelID = os.Getenv("SLACK_CHANNEL")
	}
	if *autoInvite == "" {
		*autoInvite = os.Getenv("SLACK_AUTO_INVITE")
	}

	if *botToken == "" || *appToken == "" {
		log.Fatal("Both -bot-token and -app-token are required (or set SLACK_BOT_TOKEN and SLACK_APP_TOKEN)")
	}

	// Parse auto-invite user IDs (comma-separated)
	var autoInviteUsers []string
	if *autoInvite != "" {
		for _, u := range strings.Split(*autoInvite, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				autoInviteUsers = append(autoInviteUsers, u)
			}
		}
	}

	cfg := slackbot.Config{
		BotToken:        *botToken,
		AppToken:        *appToken,
		RPCEndpoint:     *rpcURL,
		ChannelID:       *channelID,
		DynamicChannels: *dynamicChannels,
		ChannelPrefix:   *channelPrefix,
		TownRoot:        *townRoot,
		AutoInviteUsers: autoInviteUsers,
		Debug:           *debug,
	}

	bot, err := slackbot.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
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
	log.Printf("RPC endpoint: %s", *rpcURL)
	if *channelID != "" {
		log.Printf("Default notifications channel: %s", *channelID)
	}
	if *dynamicChannels {
		log.Printf("Dynamic channel creation enabled (prefix: %s)", *channelPrefix)
	}
	if len(autoInviteUsers) > 0 {
		log.Printf("Auto-invite users: %v", autoInviteUsers)
	}

	// Start SSE listener for real-time decision notifications
	if *channelID != "" {
		sseURL := *rpcURL + "/events/decisions"
		sseListener := slackbot.NewSSEListener(sseURL, bot, bot.RPCClient())
		go func() {
			log.Printf("Starting SSE listener: %s", sseURL)
			if err := sseListener.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("SSE listener error: %v", err)
			}
		}()
	}

	if err := bot.Run(ctx); err != nil {
		log.Fatalf("Bot error: %v", err)
	}
}
