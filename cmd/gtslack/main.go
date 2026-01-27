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
//	SLACK_BOT_TOKEN - Bot OAuth token (xoxb-...)
//	SLACK_APP_TOKEN - App-level token for Socket Mode (xapp-...)
//	GTMOBILE_RPC    - gtmobile RPC endpoint URL
//	SLACK_CHANNEL   - Channel ID for decision notifications
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/steveyegge/gastown/internal/slackbot"
)

var (
	botToken   = flag.String("bot-token", "", "Slack bot token (xoxb-...)")
	appToken   = flag.String("app-token", "", "Slack app token for Socket Mode (xapp-...)")
	rpcURL     = flag.String("rpc", "http://localhost:8443", "gtmobile RPC endpoint URL")
	channelID  = flag.String("channel", "", "Channel ID for decision notifications")
	debug      = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

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

	if *botToken == "" || *appToken == "" {
		log.Fatal("Both -bot-token and -app-token are required (or set SLACK_BOT_TOKEN and SLACK_APP_TOKEN)")
	}

	cfg := slackbot.Config{
		BotToken:    *botToken,
		AppToken:    *appToken,
		RPCEndpoint: *rpcURL,
		ChannelID:   *channelID,
		Debug:       *debug,
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
		log.Printf("Notifications channel: %s", *channelID)
	}

	if err := bot.Run(ctx); err != nil {
		log.Fatalf("Bot error: %v", err)
	}
}
