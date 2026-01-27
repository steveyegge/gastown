// Package slackbot implements a Slack bot for Gas Town decision management.
// It uses the slack-go/slack library with Socket Mode for WebSocket-based communication.
package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"

	"github.com/steveyegge/gastown/internal/rpcclient"
)

// Bot is a Slack bot for managing Gas Town decisions.
type Bot struct {
	client      *slack.Client
	socketMode  *socketmode.Client
	rpcClient   *rpcclient.Client
	channelID   string // Channel to post decision notifications
	debug       bool
}

// Config holds configuration for the Slack bot.
type Config struct {
	BotToken    string // xoxb-... Slack bot token
	AppToken    string // xapp-... Slack app-level token (for Socket Mode)
	RPCEndpoint string // gtmobile RPC server URL
	ChannelID   string // Channel for decision notifications
	Debug       bool
}

// New creates a new Slack bot.
func New(cfg Config) (*Bot, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("bot token is required")
	}
	if cfg.AppToken == "" {
		return nil, fmt.Errorf("app token is required for Socket Mode")
	}
	if !strings.HasPrefix(cfg.AppToken, "xapp-") {
		return nil, fmt.Errorf("app token must start with xapp-")
	}

	client := slack.New(
		cfg.BotToken,
		slack.OptionDebug(cfg.Debug),
		slack.OptionAppLevelToken(cfg.AppToken),
	)

	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(cfg.Debug),
	)

	rpcClient := rpcclient.NewClient(cfg.RPCEndpoint)

	return &Bot{
		client:     client,
		socketMode: socketClient,
		rpcClient:  rpcClient,
		channelID:  cfg.ChannelID,
		debug:      cfg.Debug,
	}, nil
}

// Run starts the bot event loop. Blocks until context is canceled.
func (b *Bot) Run(ctx context.Context) error {
	go func() {
		for evt := range b.socketMode.Events {
			b.handleEvent(evt)
		}
	}()

	go func() {
		<-ctx.Done()
		// Socket mode will be closed when Run returns
	}()

	return b.socketMode.Run()
}

func (b *Bot) handleEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		log.Println("Slack: Connecting to Socket Mode...")

	case socketmode.EventTypeConnected:
		log.Println("Slack: Connected to Socket Mode")

	case socketmode.EventTypeConnectionError:
		log.Printf("Slack: Connection error: %v", evt.Data)

	case socketmode.EventTypeSlashCommand:
		cmd, ok := evt.Data.(slack.SlashCommand)
		if !ok {
			return
		}
		b.socketMode.Ack(*evt.Request)
		b.handleSlashCommand(cmd)

	case socketmode.EventTypeInteractive:
		callback, ok := evt.Data.(slack.InteractionCallback)
		if !ok {
			return
		}
		b.socketMode.Ack(*evt.Request)
		b.handleInteraction(callback)
	}
}

func (b *Bot) handleSlashCommand(cmd slack.SlashCommand) {
	switch cmd.Command {
	case "/decisions", "/decide":
		b.handleDecisionsCommand(cmd)
	default:
		b.postEphemeral(cmd.ChannelID, cmd.UserID,
			fmt.Sprintf("Unknown command: %s", cmd.Command))
	}
}

func (b *Bot) handleDecisionsCommand(cmd slack.SlashCommand) {
	// Fetch pending decisions from RPC
	ctx := context.Background()
	decisions, err := b.rpcClient.ListPendingDecisions(ctx)
	if err != nil {
		b.postEphemeral(cmd.ChannelID, cmd.UserID,
			fmt.Sprintf("Error fetching decisions: %v", err))
		return
	}

	if len(decisions) == 0 {
		b.postEphemeral(cmd.ChannelID, cmd.UserID,
			"No pending decisions.")
		return
	}

	// Build message with decision list
	blocks := []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "Pending Decisions", false, false),
		),
	}

	for _, d := range decisions {
		// Truncate question if too long
		question := d.Question
		if len(question) > 100 {
			question = question[:97] + "..."
		}

		urgencyEmoji := ":white_circle:"
		switch d.Urgency {
		case "high":
			urgencyEmoji = ":red_circle:"
		case "medium":
			urgencyEmoji = ":large_yellow_circle:"
		case "low":
			urgencyEmoji = ":large_green_circle:"
		}

		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					fmt.Sprintf("%s *%s*\n%s", urgencyEmoji, d.ID, question),
					false, false,
				),
				nil,
				slack.NewAccessory(
					slack.NewButtonBlockElement(
						"view_decision",
						d.ID,
						slack.NewTextBlockObject("plain_text", "View", false, false),
					),
				),
			),
		)
	}

	_, _, err = b.client.PostMessage(cmd.ChannelID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionResponseURL(cmd.ResponseURL, slack.ResponseTypeEphemeral),
	)
	if err != nil {
		log.Printf("Slack: Error posting message: %v", err)
	}
}

func (b *Bot) handleInteraction(callback slack.InteractionCallback) {
	for _, action := range callback.ActionCallback.BlockActions {
		switch action.ActionID {
		case "view_decision":
			b.handleViewDecision(callback, action.Value)
		default:
			if strings.HasPrefix(action.ActionID, "resolve_") {
				b.handleResolveDecision(callback, action)
			}
		}
	}
}

func (b *Bot) handleViewDecision(callback slack.InteractionCallback, decisionID string) {
	ctx := context.Background()
	decisions, err := b.rpcClient.ListPendingDecisions(ctx)
	if err != nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Error fetching decision: %v", err))
		return
	}

	// Find the decision
	var decision *rpcclient.Decision
	for _, d := range decisions {
		if d.ID == decisionID {
			decision = &d
			break
		}
	}

	if decision == nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Decision %s not found or already resolved.", decisionID))
		return
	}

	// Build detailed decision view with option buttons
	blocks := []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "Decision Required", false, false),
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*ID:* %s\n*Question:* %s", decision.ID, decision.Question),
				false, false,
			),
			nil, nil,
		),
	}

	if decision.Context != "" {
		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					fmt.Sprintf("*Context:*\n%s", decision.Context),
					false, false,
				),
				nil, nil,
			),
		)
	}

	blocks = append(blocks, slack.NewDividerBlock())

	// Add buttons for each option
	var buttons []slack.BlockElement
	for i, opt := range decision.Options {
		label := opt.Label
		if opt.Recommended {
			label = "⭐ " + label
		}
		if len(label) > 75 {
			label = label[:72] + "..."
		}

		buttons = append(buttons,
			slack.NewButtonBlockElement(
				fmt.Sprintf("resolve_%s_%d", decision.ID, i+1),
				fmt.Sprintf("%s:%d", decision.ID, i+1),
				slack.NewTextBlockObject("plain_text", label, false, false),
			),
		)
	}

	if len(buttons) > 0 {
		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", "*Options:*", false, false),
				nil, nil,
			),
			slack.NewActionBlock("decision_actions", buttons...),
		)
	}

	_, _, err = b.client.PostMessage(callback.Channel.ID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionResponseURL(callback.ResponseURL, slack.ResponseTypeEphemeral),
	)
	if err != nil {
		log.Printf("Slack: Error posting decision view: %v", err)
	}
}

func (b *Bot) handleResolveDecision(callback slack.InteractionCallback, action *slack.BlockAction) {
	// Parse decision ID and choice from action value (format: "id:choice")
	parts := strings.Split(action.Value, ":")
	if len(parts) != 2 {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			"Invalid action value")
		return
	}

	decisionID := parts[0]
	var chosenIndex int
	fmt.Sscanf(parts[1], "%d", &chosenIndex)

	// Resolve via RPC
	ctx := context.Background()
	rationale := fmt.Sprintf("Resolved via Slack by <@%s>", callback.User.ID)

	resolved, err := b.rpcClient.ResolveDecision(ctx, decisionID, chosenIndex, rationale)
	if err != nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Error resolving decision: %v", err))
		return
	}

	// Confirm resolution
	b.postEphemeral(callback.Channel.ID, callback.User.ID,
		fmt.Sprintf("✅ Decision %s resolved! Choice: %d", resolved.ID, resolved.ChosenIndex))

	// Optionally post to channel if configured
	if b.channelID != "" && b.channelID != callback.Channel.ID {
		_, _, _ = b.client.PostMessage(b.channelID,
			slack.MsgOptionText(
				fmt.Sprintf("Decision `%s` resolved by <@%s>", decisionID, callback.User.ID),
				false,
			),
		)
	}
}

func (b *Bot) postEphemeral(channelID, userID, text string) {
	_, err := b.client.PostEphemeral(channelID, userID,
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		log.Printf("Slack: Error posting ephemeral: %v", err)
	}
}

// NotifyNewDecision posts a new decision notification to the configured channel.
func (b *Bot) NotifyNewDecision(decision rpcclient.Decision) error {
	if b.channelID == "" {
		return nil
	}

	urgencyEmoji := ":white_circle:"
	switch decision.Urgency {
	case "high":
		urgencyEmoji = ":red_circle:"
	case "medium":
		urgencyEmoji = ":large_yellow_circle:"
	case "low":
		urgencyEmoji = ":large_green_circle:"
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("%s *New Decision Required*\n*ID:* %s\n%s",
					urgencyEmoji, decision.ID, decision.Question),
				false, false,
			),
			nil,
			slack.NewAccessory(
				slack.NewButtonBlockElement(
					"view_decision",
					decision.ID,
					slack.NewTextBlockObject("plain_text", "View & Resolve", false, false),
				),
			),
		),
	}

	_, _, err := b.client.PostMessage(b.channelID,
		slack.MsgOptionBlocks(blocks...),
	)
	return err
}
