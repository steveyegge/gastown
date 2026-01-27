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
		blocks := []slack.Block{
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					"✨ *No pending decisions!*\n\nAll decisions have been resolved. Check back later or wait for notifications.",
					false, false,
				),
				nil, nil,
			),
		}
		_, _, _ = b.client.PostMessage(cmd.ChannelID,
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionResponseURL(cmd.ResponseURL, slack.ResponseTypeEphemeral),
		)
		return
	}

	// Count by urgency
	highCount, medCount, lowCount := 0, 0, 0
	for _, d := range decisions {
		switch d.Urgency {
		case "high":
			highCount++
		case "medium":
			medCount++
		default:
			lowCount++
		}
	}

	// Build message with decision list
	summaryText := fmt.Sprintf("📋 *%d Pending Decision", len(decisions))
	if len(decisions) > 1 {
		summaryText += "s"
	}
	summaryText += "*"
	if highCount > 0 {
		summaryText += fmt.Sprintf(" (:red_circle: %d high", highCount)
		if medCount > 0 || lowCount > 0 {
			summaryText += ","
		}
		if medCount > 0 {
			summaryText += fmt.Sprintf(" :large_yellow_circle: %d med", medCount)
		}
		if lowCount > 0 {
			summaryText += fmt.Sprintf(" :large_green_circle: %d low", lowCount)
		}
		summaryText += ")"
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", summaryText, false, false),
			nil, nil,
		),
		slack.NewDividerBlock(),
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
	// Handle view submissions (modal form submissions)
	if callback.Type == slack.InteractionTypeViewSubmission {
		b.handleViewSubmission(callback)
		return
	}

	// Handle block actions (button clicks, etc.)
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

	// Fetch decision details for the modal
	ctx := context.Background()
	decisions, err := b.rpcClient.ListPendingDecisions(ctx)
	if err != nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Error fetching decision: %v", err))
		return
	}

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

	// Get the selected option label
	optionLabel := fmt.Sprintf("Option %d", chosenIndex)
	if chosenIndex > 0 && chosenIndex <= len(decision.Options) {
		optionLabel = decision.Options[chosenIndex-1].Label
	}

	// Open modal for rationale input
	modalRequest := b.buildResolveModal(decisionID, chosenIndex, decision.Question, optionLabel, callback.Channel.ID)

	_, err = b.client.OpenView(callback.TriggerID, modalRequest)
	if err != nil {
		log.Printf("Slack: Error opening modal: %v", err)
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Error opening dialog: %v", err))
	}
}

func (b *Bot) buildResolveModal(decisionID string, chosenIndex int, question, optionLabel, channelID string) slack.ModalViewRequest {
	// Truncate question if too long for display
	displayQuestion := question
	if len(displayQuestion) > 200 {
		displayQuestion = displayQuestion[:197] + "..."
	}

	// Truncate option label for metadata (max ~100 chars to stay within Slack limits)
	metadataLabel := optionLabel
	if len(metadataLabel) > 100 {
		metadataLabel = metadataLabel[:97] + "..."
	}

	// Private metadata to pass through to submission (format: id:index:channel:label)
	// Using | as separator for label since it might contain colons
	metadata := fmt.Sprintf("%s:%d:%s|%s", decisionID, chosenIndex, channelID, metadataLabel)

	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      "resolve_decision_modal",
		Title:           slack.NewTextBlockObject("plain_text", "Resolve Decision", false, false),
		Submit:          slack.NewTextBlockObject("plain_text", "Resolve", false, false),
		Close:           slack.NewTextBlockObject("plain_text", "Cancel", false, false),
		PrivateMetadata: metadata,
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn",
						fmt.Sprintf("*Decision:* %s\n\n%s", decisionID, displayQuestion),
						false, false,
					),
					nil, nil,
				),
				slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn",
						fmt.Sprintf("*Selected Option:* %s", optionLabel),
						false, false,
					),
					nil, nil,
				),
				slack.NewDividerBlock(),
				slack.NewInputBlock(
					"rationale_block",
					slack.NewTextBlockObject("plain_text", "Rationale (optional)", false, false),
					slack.NewTextBlockObject("plain_text", "Explain why you chose this option", false, false),
					slack.NewPlainTextInputBlockElement(
						slack.NewTextBlockObject("plain_text", "Enter your reasoning...", false, false),
						"rationale_input",
					),
				),
			},
		},
	}
}

func (b *Bot) handleViewSubmission(callback slack.InteractionCallback) {
	if callback.View.CallbackID != "resolve_decision_modal" {
		return
	}

	// Parse private metadata (format: "decisionID:chosenIndex:channelID|optionLabel")
	metadata := callback.View.PrivateMetadata
	labelSep := strings.LastIndex(metadata, "|")
	optionLabel := ""
	if labelSep > 0 {
		optionLabel = metadata[labelSep+1:]
		metadata = metadata[:labelSep]
	}

	parts := strings.Split(metadata, ":")
	if len(parts) < 3 {
		log.Printf("Slack: Invalid modal metadata: %s", callback.View.PrivateMetadata)
		return
	}

	decisionID := parts[0]
	var chosenIndex int
	fmt.Sscanf(parts[1], "%d", &chosenIndex)
	channelID := parts[2]

	// Get rationale from form
	rationale := ""
	if rationaleBlock, ok := callback.View.State.Values["rationale_block"]; ok {
		if rationaleInput, ok := rationaleBlock["rationale_input"]; ok {
			rationale = rationaleInput.Value
		}
	}

	// Add user attribution if rationale is empty or append to existing
	userAttribution := fmt.Sprintf("Resolved via Slack by <@%s>", callback.User.ID)
	if rationale == "" {
		rationale = userAttribution
	} else {
		rationale = rationale + "\n\n— " + userAttribution
	}

	// Resolve via RPC
	ctx := context.Background()
	resolved, err := b.rpcClient.ResolveDecision(ctx, decisionID, chosenIndex, rationale)
	if err != nil {
		// Post detailed error to channel
		b.postErrorMessage(channelID, callback.User.ID, decisionID, err)
		return
	}

	// Post rich confirmation
	b.postResolutionConfirmation(channelID, callback.User.ID, resolved.ID, optionLabel, rationale)

	// Post to notification channel if configured and different
	if b.channelID != "" && b.channelID != channelID {
		b.postResolutionNotification(decisionID, optionLabel, callback.User.ID)
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

func (b *Bot) postErrorMessage(channelID, userID, decisionID string, err error) {
	errMsg := err.Error()

	// Provide specific guidance based on error type
	hint := ""
	if strings.Contains(errMsg, "not found") {
		hint = "\n\n💡 *Tip:* This decision may have already been resolved by someone else. Run `/decisions` to see current pending decisions."
	} else if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "connection") {
		hint = "\n\n💡 *Tip:* The Gas Town server may be temporarily unavailable. Please try again in a moment."
	} else if strings.Contains(errMsg, "RPC error") {
		hint = "\n\n💡 *Tip:* There was a server error. If this persists, check that gtmobile is running."
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("❌ *Failed to resolve decision*\n\n*Decision ID:* `%s`\n*Error:* %s%s",
					decisionID, errMsg, hint),
				false, false,
			),
			nil, nil,
		),
	}

	_, err2 := b.client.PostEphemeral(channelID, userID,
		slack.MsgOptionBlocks(blocks...),
	)
	if err2 != nil {
		log.Printf("Slack: Error posting error message: %v", err2)
	}
}

func (b *Bot) postResolutionConfirmation(channelID, userID, decisionID, optionLabel, rationale string) {
	// Truncate rationale for display
	displayRationale := rationale
	if len(displayRationale) > 200 {
		displayRationale = displayRationale[:197] + "..."
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("✅ *Decision Resolved Successfully!*\n\n"+
					"*Decision ID:* `%s`\n"+
					"*Your Choice:* %s\n"+
					"*Rationale:* %s",
					decisionID, optionLabel, displayRationale),
				false, false,
			),
			nil, nil,
		),
		slack.NewContextBlock("",
			slack.NewTextBlockObject("mrkdwn",
				"_The decision owner and any blocked tasks have been notified._",
				false, false,
			),
		),
	}

	_, err := b.client.PostEphemeral(channelID, userID,
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		log.Printf("Slack: Error posting confirmation: %v", err)
	}
}

func (b *Bot) postResolutionNotification(decisionID, optionLabel, userID string) {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("📋 *Decision Resolved*\n\n"+
					"*ID:* `%s`\n"+
					"*Choice:* %s\n"+
					"*Resolved by:* <@%s>",
					decisionID, optionLabel, userID),
				false, false,
			),
			nil, nil,
		),
	}

	_, _, err := b.client.PostMessage(b.channelID,
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		log.Printf("Slack: Error posting resolution notification: %v", err)
	}
}

// RPCClient returns the bot's RPC client for use by the SSE listener.
func (b *Bot) RPCClient() *rpcclient.Client {
	return b.rpcClient
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
