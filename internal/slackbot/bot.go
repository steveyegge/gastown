// Package slackbot implements a Slack bot for Gas Town decision management.
// It uses the slack-go/slack library with Socket Mode for WebSocket-based communication.
package slackbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/steveyegge/gastown/internal/rpcclient"
	slackrouter "github.com/steveyegge/gastown/internal/slack"
	"github.com/steveyegge/gastown/internal/util"
)

// messageInfo tracks a posted Slack message for later updates/deletion.
type messageInfo struct {
	channelID string
	timestamp string
}

// Bot is a Slack bot for managing Gas Town decisions.
type Bot struct {
	client      *slack.Client
	socketMode  *socketmode.Client
	rpcClient   *rpcclient.Client
	channelID   string                  // Default channel to post decision notifications
	router      *slackrouter.Router     // Channel router for per-agent routing
	debug       bool

	// Dynamic channel creation
	dynamicChannels    bool              // Enable automatic channel creation
	channelPrefix      string            // Prefix for created channels (e.g., "gt-decisions")
	channelCache       map[string]string // Cache: agent pattern → channel ID
	channelCacheMu     sync.RWMutex      // Protects channelCache

	// Decision message tracking for auto-dismiss
	decisionMessages   map[string]messageInfo // decision ID → message info
	decisionMessagesMu sync.RWMutex           // Protects decisionMessages
}

// Config holds configuration for the Slack bot.
type Config struct {
	BotToken         string // xoxb-... Slack bot token
	AppToken         string // xapp-... Slack app-level token (for Socket Mode)
	RPCEndpoint      string // gtmobile RPC server URL
	ChannelID        string // Default channel for decision notifications
	RouterConfigPath string // Optional path to slack.json for per-agent routing
	DynamicChannels  bool   // Enable automatic channel creation based on agent identity
	ChannelPrefix    string // Prefix for dynamically created channels (default: "gt-decisions")
	Debug            bool
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

	// Load channel router for per-agent routing
	var router *slackrouter.Router
	if cfg.RouterConfigPath != "" {
		var err error
		router, err = slackrouter.LoadRouterFromFile(cfg.RouterConfigPath)
		if err != nil {
			log.Printf("Slack: Warning: failed to load router from %s: %v", cfg.RouterConfigPath, err)
		} else if router.IsEnabled() {
			log.Printf("Slack: Channel router loaded from %s", cfg.RouterConfigPath)
		}
	} else {
		// Try auto-discovery from standard locations
		var err error
		router, err = slackrouter.LoadRouter()
		if err != nil {
			log.Printf("Slack: Router auto-discovery failed: %v", err)
		} else if router == nil {
			log.Printf("Slack: Router loaded but is nil")
		} else if !router.IsEnabled() {
			log.Printf("Slack: Router loaded but not enabled")
		} else {
			log.Printf("Slack: Channel router auto-loaded (enabled=%v)", router.IsEnabled())
		}
	}

	// Set default channel prefix
	channelPrefix := cfg.ChannelPrefix
	if channelPrefix == "" {
		channelPrefix = "gt-decisions"
	}

	bot := &Bot{
		client:           client,
		socketMode:       socketClient,
		rpcClient:        rpcClient,
		channelID:        cfg.ChannelID,
		router:           router,
		debug:            cfg.Debug,
		dynamicChannels:  cfg.DynamicChannels,
		channelPrefix:    channelPrefix,
		channelCache:     make(map[string]string),
		decisionMessages: make(map[string]messageInfo),
	}
	log.Printf("Slack: Bot created with router=%v", bot.router != nil)
	return bot, nil
}

// Run starts the bot event loop. Blocks until context is canceled.
func (b *Bot) Run(ctx context.Context) error {
	go func() {
		for evt := range b.socketMode.Events {
			b.handleEvent(evt)
		}
	}()

	// Auto-join all public channels on startup (gt-baeko)
	if err := b.JoinAllChannels(); err != nil {
		log.Printf("Slack: Warning: failed to auto-join channels: %v", err)
		// Continue anyway - bot can still function in channels it's already in
	}

	// Use RunContext for graceful shutdown on SIGTERM (hq-je8tm7.1)
	// RunContext closes the WebSocket when context is canceled, allowing
	// systemctl restart to complete quickly instead of waiting 90s for SIGKILL.
	return b.socketMode.RunContext(ctx)
}

func (b *Bot) handleEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		log.Println("Slack: Connecting to Socket Mode...")

	case socketmode.EventTypeConnected:
		log.Println("Slack: Connected to Socket Mode")

	case socketmode.EventTypeConnectionError:
		log.Printf("Slack: Connection error: %v", evt.Data)

	case socketmode.EventTypeEventsAPI:
		// Handle Events API callbacks (channel_created, etc.) (gt-baeko)
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		b.socketMode.Ack(*evt.Request)
		b.handleEventsAPI(eventsAPIEvent)

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

		agentTag := ""
		if d.RequestedBy != "" {
			agentTag = fmt.Sprintf(" (%s)", d.RequestedBy)
		}

		// Generate semantic slug for human-friendly display
		semanticSlug := util.GenerateDecisionSlug(d.ID, d.Question)

		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					fmt.Sprintf("%s *%s*%s\n%s", urgencyEmoji, semanticSlug, agentTag, question),
					false, false,
				),
				nil,
				slack.NewAccessory(
					slack.NewButtonBlockElement(
						"view_decision",
						d.ID, // Keep original ID for button action
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
		case "break_out":
			b.handleBreakOut(callback, action.Value)
		case "unbreak_out":
			b.handleUnbreakOut(callback, action.Value)
		case "dismiss_decision":
			b.handleDismissDecision(callback, action.Value)
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

	// Generate semantic slug for human-friendly display
	semanticSlug := util.GenerateDecisionSlug(decision.ID, decision.Question)

	// Build detailed decision view with option buttons
	blocks := []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", fmt.Sprintf("Decision: %s", semanticSlug), false, false),
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*From:* %s\n*Question:* %s", decision.RequestedBy, decision.Question),
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

	// Add options with descriptions and resolve buttons
	for i, opt := range decision.Options {
		label := opt.Label
		if opt.Recommended {
			label = "⭐ " + label
		}
		buttonLabel := label
		if len(buttonLabel) > 75 {
			buttonLabel = buttonLabel[:72] + "..."
		}

		// Show option with description
		optText := fmt.Sprintf("*%d. %s*", i+1, label)
		if opt.Description != "" {
			optText += fmt.Sprintf("\n%s", opt.Description)
		}

		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", optText, false, false),
				nil,
				slack.NewAccessory(
					slack.NewButtonBlockElement(
						fmt.Sprintf("resolve_%s_%d", decision.ID, i+1),
						fmt.Sprintf("%s:%d", decision.ID, i+1),
						slack.NewTextBlockObject("plain_text", "Choose", false, false),
					),
				),
			),
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

// handleBreakOut creates a dedicated channel for an agent and routes their decisions there.
func (b *Bot) handleBreakOut(callback slack.InteractionCallback, agent string) {
	if b.router == nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			"Break Out is not available: channel router not configured")
		return
	}

	// Check if already broken out
	if b.router.HasOverride(agent) {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Agent %s already has a dedicated channel.", agent))
		return
	}

	// Create dedicated channel name with FULL agent path
	channelName := b.agentToBreakOutChannelName(agent)

	// Find or create the channel
	channelID, err := b.ensureBreakOutChannelExists(agent, channelName)
	if err != nil {
		log.Printf("Slack: Break Out failed for %s: %v", agent, err)
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Failed to create dedicated channel: %v", err))
		return
	}

	// Add override to router
	b.router.AddOverrideWithName(agent, channelID, channelName)

	// Save the config
	if err := b.router.Save(); err != nil {
		log.Printf("Slack: Failed to save router config after Break Out: %v", err)
		// Continue anyway - override is active in memory
	}

	log.Printf("Slack: Break Out: %s → #%s (%s)", agent, channelName, channelID)

	// Repost pending decisions for this agent to the new channel
	ctx := context.Background()
	decisions, err := b.rpcClient.ListPendingDecisions(ctx)
	if err != nil {
		log.Printf("Slack: Break Out: failed to fetch pending decisions: %v", err)
	} else {
		repostedCount := 0
		for _, d := range decisions {
			if d.RequestedBy == agent {
				if err := b.notifyDecisionToChannel(d, channelID); err != nil {
					log.Printf("Slack: Break Out: failed to repost decision %s: %v", d.ID, err)
				} else {
					repostedCount++
				}
			}
		}
		if repostedCount > 0 {
			log.Printf("Slack: Break Out: reposted %d pending decision(s) to new channel", repostedCount)
		}
	}

	b.postEphemeral(callback.Channel.ID, callback.User.ID,
		fmt.Sprintf("✅ Broke out *%s* to dedicated channel <#%s>. Future decisions will go there.", agent, channelID))
}

// handleUnbreakOut removes the dedicated channel override for an agent.
func (b *Bot) handleUnbreakOut(callback slack.InteractionCallback, agent string) {
	if b.router == nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			"Unbreak Out is not available: channel router not configured")
		return
	}

	// Check if actually broken out
	if !b.router.HasOverride(agent) {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Agent %s doesn't have a dedicated channel override.", agent))
		return
	}

	// Remove the override
	prevChannel := b.router.RemoveOverride(agent)

	// Save the config
	if err := b.router.Save(); err != nil {
		log.Printf("Slack: Failed to save router config after Unbreak Out: %v", err)
		// Continue anyway - override is removed in memory
	}

	// Resolve what channel they'll go to now
	result := b.router.Resolve(agent)
	newChannel := result.ChannelID

	log.Printf("Slack: Unbreak Out: %s removed override %s, now routes to %s", agent, prevChannel, newChannel)
	b.postEphemeral(callback.Channel.ID, callback.User.ID,
		fmt.Sprintf("✅ Unbroke out *%s*. Future decisions will go to <#%s>.", agent, newChannel))
}

// handleDismissDecision deletes the decision notification from Slack.
// The decision itself remains in the system but is removed from chat.
func (b *Bot) handleDismissDecision(callback slack.InteractionCallback, decisionID string) {
	// Get the message timestamp from the callback
	messageTs := callback.Message.Timestamp
	if messageTs == "" {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			"Could not dismiss: message timestamp not found")
		return
	}

	// Delete the message
	_, _, err := b.client.DeleteMessage(callback.Channel.ID, messageTs)
	if err != nil {
		log.Printf("Slack: Failed to delete decision message %s: %v", decisionID, err)
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Failed to dismiss: %v", err))
		return
	}

	log.Printf("Slack: Dismissed decision %s (deleted message %s)", decisionID, messageTs)
	// No confirmation needed - the message disappearing IS the confirmation
}

// DismissDecisionByID deletes a decision's Slack notification message by decision ID.
// Used for auto-dismissing stale or cancelled decisions.
// Returns true if the message was found and deleted, false otherwise.
func (b *Bot) DismissDecisionByID(decisionID string) bool {
	b.decisionMessagesMu.RLock()
	msgInfo, found := b.decisionMessages[decisionID]
	b.decisionMessagesMu.RUnlock()

	if !found {
		return false
	}

	_, _, err := b.client.DeleteMessage(msgInfo.channelID, msgInfo.timestamp)
	if err != nil {
		log.Printf("Slack: Failed to auto-dismiss decision %s: %v", decisionID, err)
		return false
	}

	// Remove from tracking
	b.decisionMessagesMu.Lock()
	delete(b.decisionMessages, decisionID)
	b.decisionMessagesMu.Unlock()

	log.Printf("Slack: Auto-dismissed decision %s (deleted message %s)", decisionID, msgInfo.timestamp)
	return true
}

// agentToBreakOutChannelName converts an agent identity to a dedicated Break Out channel name.
// Unlike agentToChannelName, this includes the FULL agent path for dedicated channels.
// Examples:
//   - "gastown/crew/slack_decisions" → "gt-decisions-gastown-crew-slack_decisions"
//   - "gastown/polecats/furiosa" → "gt-decisions-gastown-polecats-furiosa"
func (b *Bot) agentToBreakOutChannelName(agent string) string {
	parts := strings.Split(agent, "/")

	// Use FULL agent path for dedicated break-out channels
	name := b.channelPrefix + "-" + strings.Join(parts, "-")

	// Sanitize for Slack: lowercase, replace invalid chars with hyphens
	name = strings.ToLower(name)
	name = regexp.MustCompile(`[^a-z0-9-_]`).ReplaceAllString(name, "-")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-") // collapse multiple hyphens
	name = strings.Trim(name, "-")

	// Slack channel names max 80 chars
	if len(name) > 80 {
		name = name[:80]
	}

	return name
}

// ensureBreakOutChannelExists finds or creates a dedicated Break Out channel.
func (b *Bot) ensureBreakOutChannelExists(agent, channelName string) (string, error) {
	// Check cache first
	b.channelCacheMu.RLock()
	if cachedID, ok := b.channelCache[channelName]; ok {
		b.channelCacheMu.RUnlock()
		return cachedID, nil
	}
	b.channelCacheMu.RUnlock()

	// Look up channel by name
	channelID, err := b.findChannelByName(channelName)
	if err == nil && channelID != "" {
		b.cacheChannel(channelName, channelID)
		log.Printf("Slack: Found existing Break Out channel #%s (%s) for agent %s", channelName, channelID, agent)
		return channelID, nil
	}

	// Create the channel
	channel, err := b.client.CreateConversation(slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   false,
	})
	if err != nil {
		// Check if it's a "name_taken" error (channel exists but we couldn't find it)
		if strings.Contains(err.Error(), "name_taken") {
			channelID, findErr := b.findChannelByName(channelName)
			if findErr == nil && channelID != "" {
				b.cacheChannel(channelName, channelID)
				return channelID, nil
			}
		}
		return "", fmt.Errorf("create channel %s: %w", channelName, err)
	}

	b.cacheChannel(channelName, channel.ID)
	log.Printf("Slack: Created Break Out channel #%s (%s) for agent %s", channelName, channel.ID, agent)
	return channel.ID, nil
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
	_, _ = fmt.Sscanf(parts[1], "%d", &chosenIndex)

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
	// Pass message timestamp so we can edit the original message on resolution
	messageTs := callback.Message.Timestamp
	modalRequest := b.buildResolveModal(decisionID, chosenIndex, decision.Question, optionLabel, callback.Channel.ID, messageTs)

	_, err = b.client.OpenView(callback.TriggerID, modalRequest)
	if err != nil {
		log.Printf("Slack: Error opening modal: %v", err)
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Error opening dialog: %v", err))
	}
}

func (b *Bot) buildResolveModal(decisionID string, chosenIndex int, question, optionLabel, channelID, messageTs string) slack.ModalViewRequest {
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

	// Private metadata to pass through to submission (format: id:index:channel:messageTs|label)
	// Using | as separator for label since it might contain colons
	metadata := fmt.Sprintf("%s:%d:%s:%s|%s", decisionID, chosenIndex, channelID, messageTs, metadataLabel)

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
				func() *slack.InputBlock {
					ib := slack.NewInputBlock(
						"rationale_block",
						slack.NewTextBlockObject("plain_text", "Rationale", false, false),
						slack.NewTextBlockObject("plain_text", "Optionally explain your reasoning", false, false),
						slack.NewPlainTextInputBlockElement(
							slack.NewTextBlockObject("plain_text", "Enter your reasoning...", false, false),
							"rationale_input",
						),
					)
					ib.Optional = true
					return ib
				}(),
			},
		},
	}
}

func (b *Bot) handleViewSubmission(callback slack.InteractionCallback) {
	if callback.View.CallbackID != "resolve_decision_modal" {
		return
	}

	// Parse private metadata (format: "decisionID:chosenIndex:channelID:messageTs|optionLabel")
	metadata := callback.View.PrivateMetadata
	labelSep := strings.LastIndex(metadata, "|")
	optionLabel := ""
	if labelSep > 0 {
		optionLabel = metadata[labelSep+1:]
		metadata = metadata[:labelSep]
	}

	parts := strings.Split(metadata, ":")
	if len(parts) < 4 {
		log.Printf("Slack: Invalid modal metadata: %s", callback.View.PrivateMetadata)
		return
	}

	decisionID := parts[0]
	var chosenIndex int
	_, _ = fmt.Sscanf(parts[1], "%d", &chosenIndex)
	channelID := parts[2]
	messageTs := parts[3]

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

	// Resolve via RPC with Slack user identity
	ctx := context.Background()
	resolvedBy := fmt.Sprintf("slack:%s", callback.User.ID)
	resolved, err := b.rpcClient.ResolveDecision(ctx, decisionID, chosenIndex, rationale, resolvedBy)
	if err != nil {
		// Post detailed error to channel
		b.postErrorMessage(channelID, callback.User.ID, decisionID, err)
		return
	}

	// Edit the original message to show resolved state (collapsed view)
	if messageTs != "" {
		b.updateMessageAsResolved(channelID, messageTs, resolved.ID, resolved.Question, resolved.Context, optionLabel, rationale, callback.User.ID)
	} else {
		// Fallback: post new message if we don't have the original timestamp
		b.postResolutionConfirmation(channelID, callback.User.ID, resolved.ID, optionLabel, rationale)
	}

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

// updateMessageAsResolved edits the original decision notification to show a collapsed resolved view.
// This keeps one message per decision instead of creating a new confirmation message.
// The resolverID can be a Slack user ID (will be formatted as mention) or a plain string.
func (b *Bot) updateMessageAsResolved(channelID, messageTs, decisionID, question, context, chosenOption, rationale, resolverID string) {
	// Generate semantic slug for display
	semanticSlug := util.GenerateDecisionSlug(decisionID, question)

	// Format resolver - if it looks like a Slack user ID (no colons, alphanumeric), use mention
	// Otherwise display as plain text
	resolverText := resolverID
	if resolverID != "" && !strings.Contains(resolverID, ":") && len(resolverID) > 3 {
		// Likely a Slack user ID (e.g., "U12345678")
		resolverText = fmt.Sprintf("<@%s>", resolverID)
	}

	// Build resolved text with question and context preserved
	resolvedText := fmt.Sprintf("✅ *%s* — Resolved\n\n", semanticSlug)
	resolvedText += fmt.Sprintf("*Question:* %s\n", question)
	if context != "" {
		resolvedText += fmt.Sprintf("*Context:* %s\n", context)
	}
	resolvedText += fmt.Sprintf("\n*Choice:* %s\n", chosenOption)
	if rationale != "" {
		// Truncate long rationales for display
		displayRationale := rationale
		if len(displayRationale) > 300 {
			displayRationale = displayRationale[:297] + "..."
		}
		resolvedText += fmt.Sprintf("*Rationale:* %s\n", displayRationale)
	}
	resolvedText += fmt.Sprintf("*By:* %s", resolverText)

	// Build collapsed resolved view
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", resolvedText, false, false),
			nil, nil,
		),
	}

	_, _, _, err := b.client.UpdateMessage(channelID, messageTs,
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		log.Printf("Slack: Failed to update message as resolved: %v", err)
		// Fallback: post ephemeral confirmation (only works if resolverID is a valid Slack user)
		if resolverID != "" && !strings.Contains(resolverID, ":") && len(resolverID) > 3 {
			b.postEphemeral(channelID, resolverID,
				fmt.Sprintf("✅ Decision resolved: %s", chosenOption))
		}
	} else {
		log.Printf("Slack: Updated decision %s message to resolved state", decisionID)
	}
}

func (b *Bot) postResolutionConfirmation(channelID, userID, _, optionLabel, rationale string) {
	// Truncate rationale for display
	displayRationale := rationale
	if len(displayRationale) > 200 {
		displayRationale = displayRationale[:197] + "..."
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("✅ *Decision Resolved Successfully!*\n\n"+
					"*Choice:* %s\n"+
					"*Rationale:* %s",
					optionLabel, displayRationale),
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

func (b *Bot) postResolutionNotification(_, optionLabel, userID string) {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("📋 *Decision Resolved*\n\n"+
					"*Choice:* %s\n"+
					"*Resolved by:* <@%s>",
					optionLabel, userID),
				false, false,
			),
			nil, nil,
		),
		slack.NewContextBlock("",
			slack.NewTextBlockObject("mrkdwn",
				"_The requesting agent has been notified via mail and nudge. Blocked work has been unblocked._",
				false, false,
			),
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

// resolveChannel determines the appropriate channel for an agent.
// Priority order:
// 1. Static router config (if available and matches)
// 2. Dynamic channel creation (if enabled)
// 3. Default channelID
func (b *Bot) resolveChannel(agent string) string {
	// Try static router first
	if b.router != nil && b.router.IsEnabled() && agent != "" {
		result := b.router.Resolve(agent)
		if result != nil && result.ChannelID != "" && !result.IsDefault {
			if b.debug {
				log.Printf("Slack: Routing %s to channel %s (matched by: %s)",
					agent, result.ChannelID, result.MatchedBy)
			}
			return result.ChannelID
		}
	}

	// Try dynamic channel creation
	if b.dynamicChannels && agent != "" {
		channelID, err := b.ensureChannelExists(agent)
		if err != nil {
			log.Printf("Slack: Failed to ensure channel for %s: %v (falling back to default)", agent, err)
		} else if channelID != "" {
			return channelID
		}
	}

	return b.channelID
}

// agentToChannelName converts an agent identity to a Slack channel name.
// Examples:
//   - "gastown/polecats/furiosa" → "gt-decisions-gastown-polecats"
//   - "beads/crew/wolf" → "gt-decisions-beads-crew"
//   - "mayor" → "gt-decisions-mayor"
//
// Channel names are limited to 80 chars, lowercase, no spaces, hyphens allowed.
func (b *Bot) agentToChannelName(agent string) string {
	parts := strings.Split(agent, "/")

	// Use rig and role, drop the agent name for grouping
	// gastown/polecats/furiosa → gastown-polecats
	var nameParts []string
	if len(parts) >= 2 {
		nameParts = parts[:2] // rig and role
	} else {
		nameParts = parts // single segment like "mayor"
	}

	// Build channel name
	name := b.channelPrefix + "-" + strings.Join(nameParts, "-")

	// Sanitize for Slack: lowercase, replace invalid chars with hyphens
	name = strings.ToLower(name)
	name = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(name, "-")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-") // collapse multiple hyphens
	name = strings.Trim(name, "-")

	// Slack channel names max 80 chars
	if len(name) > 80 {
		name = name[:80]
	}

	return name
}

// ensureChannelExists looks up or creates a channel for the given agent.
// Returns the channel ID.
func (b *Bot) ensureChannelExists(agent string) (string, error) {
	channelName := b.agentToChannelName(agent)

	// Check cache first
	b.channelCacheMu.RLock()
	if cachedID, ok := b.channelCache[channelName]; ok {
		b.channelCacheMu.RUnlock()
		return cachedID, nil
	}
	b.channelCacheMu.RUnlock()

	// Look up channel by name
	channelID, err := b.findChannelByName(channelName)
	if err == nil && channelID != "" {
		b.cacheChannel(channelName, channelID)
		if b.debug {
			log.Printf("Slack: Found existing channel #%s (%s) for agent %s", channelName, channelID, agent)
		}
		return channelID, nil
	}

	// Create the channel
	channel, err := b.client.CreateConversation(slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   false,
	})
	if err != nil {
		// Check if it's a "name_taken" error (channel exists but we couldn't find it)
		if strings.Contains(err.Error(), "name_taken") {
			// Try to find it again
			channelID, findErr := b.findChannelByName(channelName)
			if findErr == nil && channelID != "" {
				b.cacheChannel(channelName, channelID)
				return channelID, nil
			}
		}
		return "", fmt.Errorf("create channel %s: %w", channelName, err)
	}

	b.cacheChannel(channelName, channel.ID)
	log.Printf("Slack: Created channel #%s (%s) for agent %s", channelName, channel.ID, agent)
	return channel.ID, nil
}

// findChannelByName searches for a channel by name.
func (b *Bot) findChannelByName(name string) (string, error) {
	var cursor string
	for {
		params := &slack.GetConversationsParameters{
			Types:           []string{"public_channel"},
			Limit:           200,
			Cursor:          cursor,
			ExcludeArchived: true,
		}

		channels, nextCursor, err := b.client.GetConversations(params)
		if err != nil {
			return "", err
		}

		for _, ch := range channels {
			if ch.Name == name {
				return ch.ID, nil
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return "", nil // Not found
}

// cacheChannel stores a channel name → ID mapping.
func (b *Bot) cacheChannel(name, id string) {
	b.channelCacheMu.Lock()
	b.channelCache[name] = id
	b.channelCacheMu.Unlock()
}

// NotifyResolution updates or posts a resolution notification for a decision.
// This is called by the SSE listener when a decision is resolved externally.
// If we have a tracked message, it updates the existing message; otherwise posts a new one.
func (b *Bot) NotifyResolution(decision rpcclient.Decision) error {
	// If resolved via Slack, skip - we already edited the original message
	if strings.HasPrefix(decision.ResolvedBy, "slack:") {
		log.Printf("Slack: Skipping resolution notification for %s (resolved via Slack, message already updated)", decision.ID)
		return nil
	}

	optionLabel := "Unknown"
	if decision.ChosenIndex > 0 && decision.ChosenIndex <= len(decision.Options) {
		optionLabel = decision.Options[decision.ChosenIndex-1].Label
	}

	resolvedBy := decision.ResolvedBy
	if resolvedBy == "" {
		resolvedBy = "unknown"
	}

	// Check if we have a tracked message to update
	b.decisionMessagesMu.RLock()
	msgInfo, hasTrackedMessage := b.decisionMessages[decision.ID]
	b.decisionMessagesMu.RUnlock()

	if hasTrackedMessage {
		// Update the existing message instead of posting a new one
		b.updateMessageAsResolved(msgInfo.channelID, msgInfo.timestamp, decision.ID, decision.Question, decision.Context, optionLabel, decision.Rationale, resolvedBy)
		// Remove from tracking
		b.decisionMessagesMu.Lock()
		delete(b.decisionMessages, decision.ID)
		b.decisionMessagesMu.Unlock()
		return nil
	}

	// Fallback: post a new message if we don't have the original tracked
	targetChannel := b.resolveChannel(decision.RequestedBy)
	if targetChannel == "" {
		return nil
	}

	// Format resolver - if it's a Slack user ID, use mention; otherwise use plain text
	resolverText := resolvedBy
	if strings.HasPrefix(resolvedBy, "slack:") {
		// Extract Slack user ID and format as mention
		resolverText = fmt.Sprintf("<@%s>", strings.TrimPrefix(resolvedBy, "slack:"))
	}

	// Generate semantic slug for human-friendly display
	semanticSlug := util.GenerateDecisionSlug(decision.ID, decision.Question)

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("📋 *Decision Resolved: %s*\n\n"+
					"*Question:* %s\n"+
					"*Choice:* %s\n"+
					"*Resolved by:* %s",
					semanticSlug, decision.Question, optionLabel, resolverText),
				false, false,
			),
			nil, nil,
		),
		slack.NewContextBlock("",
			slack.NewTextBlockObject("mrkdwn",
				"_The requesting agent has been notified via mail and nudge. Blocked work has been unblocked._",
				false, false,
			),
		),
	}

	_, _, err := b.client.PostMessage(targetChannel,
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		log.Printf("Slack: Error posting resolution notification: %v", err)
		return err
	}
	return nil
}

// NotifyNewDecision posts a new decision notification to the appropriate channel.
// Uses the channel router to determine the target channel based on the requesting agent.
func (b *Bot) NotifyNewDecision(decision rpcclient.Decision) error {
	// Resolve channel based on agent identity
	targetChannel := b.resolveChannel(decision.RequestedBy)
	if targetChannel == "" {
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

	// Include agent name if available
	agentInfo := ""
	if decision.RequestedBy != "" {
		agentInfo = fmt.Sprintf(" from *%s*", decision.RequestedBy)
	}

	// Generate semantic slug for human-friendly display
	semanticSlug := util.GenerateDecisionSlug(decision.ID, decision.Question)

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("%s *%s*%s\n%s",
					urgencyEmoji, semanticSlug, agentInfo, decision.Question),
				false, false,
			),
			nil,
			slack.NewAccessory(
				slack.NewButtonBlockElement(
					"view_decision",
					decision.ID, // Keep original ID for button action
					slack.NewTextBlockObject("plain_text", "View Details", false, false),
				),
			),
		),
	}

	// Show predecessor chain info if present
	if decision.PredecessorID != "" {
		chainInfo := buildChainInfoText(decision.PredecessorID)
		blocks = append(blocks,
			slack.NewContextBlock("",
				slack.NewTextBlockObject("mrkdwn", chainInfo, false, false),
			),
		)
	}

	// Show context inline if available (with JSON formatting)
	if decision.Context != "" {
		contextText := formatContextForSlack(decision.Context, 500)
		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", contextText, false, false),
				nil, nil,
			),
		)
	}

	// Show options inline with resolve buttons (gt-1bc64)
	// This allows users to resolve directly from the notification without extra clicks
	if len(decision.Options) > 0 {
		blocks = append(blocks, slack.NewDividerBlock())

		for i, opt := range decision.Options {
			label := opt.Label
			if opt.Recommended {
				label = "⭐ " + label
			}

			// Build option text with description if available
			optText := fmt.Sprintf("*%d. %s*", i+1, label)
			if opt.Description != "" {
				// Truncate long descriptions
				desc := opt.Description
				if len(desc) > 150 {
					desc = desc[:147] + "..."
				}
				optText += fmt.Sprintf("\n%s", desc)
			}

			// Truncate button label if too long (Slack limit)
			buttonLabel := "Choose"
			if len(decision.Options) <= 4 {
				// For small option counts, show option number
				buttonLabel = fmt.Sprintf("Choose %d", i+1)
			}

			blocks = append(blocks,
				slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn", optText, false, false),
					nil,
					slack.NewAccessory(
						slack.NewButtonBlockElement(
							fmt.Sprintf("resolve_%s_%d", decision.ID, i+1),
							fmt.Sprintf("%s:%d", decision.ID, i+1),
							slack.NewTextBlockObject("plain_text", buttonLabel, false, false),
						),
					),
				),
			)
		}
	}

	// Add action buttons: Dismiss, and Break Out / Unbreak Out
	dismissButton := slack.NewButtonBlockElement(
		"dismiss_decision",
		decision.ID,
		slack.NewTextBlockObject("plain_text", "🗑️ Dismiss", false, false),
	)

	if decision.RequestedBy != "" {
		var breakOutButton *slack.ButtonBlockElement
		if b.router != nil && b.router.HasOverride(decision.RequestedBy) {
			breakOutButton = slack.NewButtonBlockElement(
				"unbreak_out",
				decision.RequestedBy,
				slack.NewTextBlockObject("plain_text", "🔀 Unbreak Out", false, false),
			)
		} else {
			breakOutButton = slack.NewButtonBlockElement(
				"break_out",
				decision.RequestedBy,
				slack.NewTextBlockObject("plain_text", "🔀 Break Out", false, false),
			)
		}
		blocks = append(blocks,
			slack.NewActionBlock("",
				dismissButton,
				breakOutButton,
			),
		)
	} else {
		// No agent identity - just show dismiss
		blocks = append(blocks,
			slack.NewActionBlock("",
				dismissButton,
			),
		)
	}

	_, ts, err := b.client.PostMessage(targetChannel,
		slack.MsgOptionBlocks(blocks...),
	)
	if err == nil && ts != "" {
		// Track the message for auto-dismiss
		b.decisionMessagesMu.Lock()
		b.decisionMessages[decision.ID] = messageInfo{
			channelID: targetChannel,
			timestamp: ts,
		}
		b.decisionMessagesMu.Unlock()
	}
	return err
}

// notifyDecisionToChannel posts a decision notification to a specific channel.
// Used by Break Out to repost pending decisions to the new dedicated channel.
func (b *Bot) notifyDecisionToChannel(decision rpcclient.Decision, channelID string) error {
	urgencyEmoji := ":white_circle:"
	switch decision.Urgency {
	case "high":
		urgencyEmoji = ":red_circle:"
	case "medium":
		urgencyEmoji = ":large_yellow_circle:"
	case "low":
		urgencyEmoji = ":large_green_circle:"
	}

	agentInfo := ""
	if decision.RequestedBy != "" {
		agentInfo = fmt.Sprintf(" from *%s*", decision.RequestedBy)
	}

	semanticSlug := util.GenerateDecisionSlug(decision.ID, decision.Question)

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("%s *%s*%s\n%s",
					urgencyEmoji, semanticSlug, agentInfo, decision.Question),
				false, false,
			),
			nil,
			slack.NewAccessory(
				slack.NewButtonBlockElement(
					"view_decision",
					decision.ID,
					slack.NewTextBlockObject("plain_text", "View Details", false, false),
				),
			),
		),
	}

	// Show predecessor chain info if present
	if decision.PredecessorID != "" {
		chainInfo := buildChainInfoText(decision.PredecessorID)
		blocks = append(blocks,
			slack.NewContextBlock("",
				slack.NewTextBlockObject("mrkdwn", chainInfo, false, false),
			),
		)
	}

	// Show context inline with JSON formatting
	if decision.Context != "" {
		contextText := formatContextForSlack(decision.Context, 500)
		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", contextText, false, false),
				nil, nil,
			),
		)
	}

	if len(decision.Options) > 0 {
		blocks = append(blocks, slack.NewDividerBlock())

		for i, opt := range decision.Options {
			label := opt.Label
			if opt.Recommended {
				label = "⭐ " + label
			}

			optText := fmt.Sprintf("*%d. %s*", i+1, label)
			if opt.Description != "" {
				desc := opt.Description
				if len(desc) > 150 {
					desc = desc[:147] + "..."
				}
				optText += fmt.Sprintf("\n%s", desc)
			}

			buttonLabel := "Choose"
			if len(decision.Options) <= 4 {
				buttonLabel = fmt.Sprintf("Choose %d", i+1)
			}

			blocks = append(blocks,
				slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn", optText, false, false),
					nil,
					slack.NewAccessory(
						slack.NewButtonBlockElement(
							fmt.Sprintf("resolve_%s_%d", decision.ID, i+1),
							fmt.Sprintf("%s:%d", decision.ID, i+1),
							slack.NewTextBlockObject("plain_text", buttonLabel, false, false),
						),
					),
				),
			)
		}
	}

	// Show Dismiss and Unbreak Out buttons (since we're in a break-out channel)
	dismissButton := slack.NewButtonBlockElement(
		"dismiss_decision",
		decision.ID,
		slack.NewTextBlockObject("plain_text", "🗑️ Dismiss", false, false),
	)

	if decision.RequestedBy != "" {
		blocks = append(blocks,
			slack.NewActionBlock("",
				dismissButton,
				slack.NewButtonBlockElement(
					"unbreak_out",
					decision.RequestedBy,
					slack.NewTextBlockObject("plain_text", "🔀 Unbreak Out", false, false),
				),
			),
		)
	} else {
		blocks = append(blocks,
			slack.NewActionBlock("",
				dismissButton,
			),
		)
	}

	_, ts, err := b.client.PostMessage(channelID,
		slack.MsgOptionBlocks(blocks...),
	)
	if err == nil && ts != "" {
		// Track the message for auto-dismiss
		b.decisionMessagesMu.Lock()
		b.decisionMessages[decision.ID] = messageInfo{
			channelID: channelID,
			timestamp: ts,
		}
		b.decisionMessagesMu.Unlock()
	}
	return err
}

// handleEventsAPI processes Events API callbacks like channel_created (gt-baeko).
func (b *Bot) handleEventsAPI(event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.ChannelCreatedEvent:
			b.handleChannelCreated(ev)
		}
	}
}

// handleChannelCreated auto-joins newly created channels (gt-baeko).
func (b *Bot) handleChannelCreated(event *slackevents.ChannelCreatedEvent) {
	channelID := event.Channel.ID
	channelName := event.Channel.Name

	log.Printf("Slack: New channel created: #%s (%s), attempting to join", channelName, channelID)

	_, _, _, err := b.client.JoinConversation(channelID)
	if err != nil {
		log.Printf("Slack: Failed to join new channel #%s: %v", channelName, err)
		return
	}

	log.Printf("Slack: Successfully joined new channel #%s", channelName)
}

// JoinAllChannels lists all public channels and joins those the bot isn't in (gt-baeko).
// This is called on startup to ensure the bot is present in all channels.
func (b *Bot) JoinAllChannels() error {
	log.Println("Slack: Auto-joining all public channels...")

	var cursor string
	joinedCount := 0
	alreadyMemberCount := 0

	for {
		// List public channels with pagination
		params := &slack.GetConversationsParameters{
			Types:           []string{"public_channel"},
			Limit:           200,
			Cursor:          cursor,
			ExcludeArchived: true,
		}

		channels, nextCursor, err := b.client.GetConversations(params)
		if err != nil {
			return fmt.Errorf("failed to list channels: %w", err)
		}

		for _, ch := range channels {
			if ch.IsMember {
				alreadyMemberCount++
				continue
			}

			// Try to join the channel
			_, _, _, err := b.client.JoinConversation(ch.ID)
			if err != nil {
				// Log but don't fail - some channels may have restrictions
				log.Printf("Slack: Could not join #%s: %v", ch.Name, err)
				continue
			}

			joinedCount++
			if b.debug {
				log.Printf("Slack: Joined #%s", ch.Name)
			}
		}

		// Check for more pages
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	log.Printf("Slack: Auto-join complete: joined %d channels, already member of %d", joinedCount, alreadyMemberCount)
	return nil
}

// formatContextForSlack formats JSON context for Slack display.
// If context is valid JSON, it pretty-prints it in a code block.
// Otherwise returns the context as-is (truncated if needed).
func formatContextForSlack(context string, maxLen int) string {
	if context == "" {
		return ""
	}

	// Try to parse as JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(context), &parsed); err != nil {
		// Not valid JSON, return truncated plain text
		if len(context) > maxLen {
			return context[:maxLen-3] + "..."
		}
		return context
	}

	// Check for successor_schemas - extract and display separately
	var schemaInfo string
	if obj, ok := parsed.(map[string]interface{}); ok {
		if _, hasSchemas := obj["successor_schemas"]; hasSchemas {
			schemaInfo = "\n📋 _Has successor schemas defined_"
			// Remove from main display
			delete(obj, "successor_schemas")
			if len(obj) == 0 {
				return schemaInfo
			}
			parsed = obj
		}
	}

	// Pretty-print JSON
	prettyJSON, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		// Fallback to truncated original
		if len(context) > maxLen {
			return context[:maxLen-3] + "..."
		}
		return context
	}

	// Format as Slack code block
	formatted := "```\n" + string(prettyJSON) + "\n```"
	if len(formatted) > maxLen {
		// Truncate while preserving code block format
		truncated := string(prettyJSON)
		maxContent := maxLen - 10 // Account for code block markers
		if len(truncated) > maxContent {
			truncated = truncated[:maxContent-3] + "..."
		}
		formatted = "```\n" + truncated + "\n```"
	}

	return formatted + schemaInfo
}

// buildChainInfoText builds a text description of predecessor chain if present.
func buildChainInfoText(predecessorID string) string {
	if predecessorID == "" {
		return ""
	}
	return fmt.Sprintf("🔗 _Chained from: %s_", predecessorID)
}

