// Package slack implements the Slack Bot integration for faultline.
// It sends DMs to linked Slack users via the chat.postMessage API
// and receives Slack interactions via a webhook endpoint.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/outdoorsea/faultline/internal/integrations"
	"github.com/outdoorsea/faultline/internal/notify"
)

const (
	slackAPIURL = "https://slack.com/api/chat.postMessage"
)

// Config holds per-project Slack bot configuration.
type Config struct {
	// BotToken is the Slack Bot User OAuth Token (xoxb-...).
	// If empty, the global FAULTLINE_SLACK_BOT_TOKEN is used.
	BotToken string `json:"bot_token,omitempty"`

	// DefaultChannelID is an optional channel to post to when no user mapping exists.
	DefaultChannelID string `json:"default_channel_id,omitempty"`
}

// Bot is the Slack integration that sends DMs via chat.postMessage.
type Bot struct {
	config Config
	client *http.Client
}

func init() {
	integrations.Register(integrations.TypeSlackBot, newSlackBot)
}

func newSlackBot(raw json.RawMessage) (integrations.Integration, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("slack bot config: %w", err)
	}
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("slack bot: bot_token is required")
	}
	return &Bot{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (b *Bot) Type() integrations.IntegrationType { return integrations.TypeSlackBot }

func (b *Bot) OnNewIssue(ctx context.Context, event notify.Event) error {
	if b.config.DefaultChannelID == "" {
		return nil // no channel configured, DMs will be wired in fl-4b0.14
	}
	return b.postMessage(ctx, b.config.DefaultChannelID, b.newIssueBlocks(event))
}

func (b *Bot) OnResolved(ctx context.Context, event notify.Event) error {
	if b.config.DefaultChannelID == "" {
		return nil
	}
	return b.postMessage(ctx, b.config.DefaultChannelID, b.resolvedBlocks(event))
}

func (b *Bot) OnRegression(ctx context.Context, event notify.Event) error {
	if b.config.DefaultChannelID == "" {
		return nil
	}
	return b.postMessage(ctx, b.config.DefaultChannelID, b.regressionBlocks(event))
}

// SendDM sends a direct message to a Slack user by their user ID.
func (b *Bot) SendDM(ctx context.Context, slackUserID string, text string) error {
	blocks := []block{
		{Type: "section", Text: &textObj{Type: "mrkdwn", Text: text}},
	}
	return b.postMessage(ctx, slackUserID, blocks)
}

// postMessage sends a message to a Slack channel or user via chat.postMessage.
func (b *Bot) postMessage(ctx context.Context, channel string, blocks []block) error {
	payload := map[string]interface{}{
		"channel": channel,
		"blocks":  blocks,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack bot: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, slackAPIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack bot: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+b.config.BotToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack bot: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack bot: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Check Slack API response for ok field.
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && !result.OK {
		return fmt.Errorf("slack bot: API error: %s", result.Error)
	}
	return nil
}

// Block Kit types for Slack messages.
type block struct {
	Type     string    `json:"type"`
	Text     *textObj  `json:"text,omitempty"`
	Fields   []textObj `json:"fields,omitempty"`
	Elements []block   `json:"elements,omitempty"`
}

type textObj struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func severityEmoji(level string) string {
	switch level {
	case "fatal":
		return ":rotating_light:"
	case "error":
		return ":warning:"
	default:
		return ":information_source:"
	}
}

func (b *Bot) newIssueBlocks(event notify.Event) []block {
	emoji := severityEmoji(event.Level)
	header := fmt.Sprintf("%s *New Fault* — %s", emoji, event.Title)

	fields := []textObj{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Level*\n%s", event.Level)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Events*\n%d", event.EventCount)},
	}
	if event.Culprit != "" {
		fields = append(fields, textObj{Type: "mrkdwn", Text: fmt.Sprintf("*Culprit*\n`%s`", event.Culprit)})
	}

	return []block{
		{Type: "section", Text: &textObj{Type: "mrkdwn", Text: header}},
		{Type: "section", Fields: fields},
	}
}

func (b *Bot) resolvedBlocks(event notify.Event) []block {
	header := fmt.Sprintf(":white_check_mark: *Fault Resolved* — %s", event.Title)
	return []block{
		{Type: "section", Text: &textObj{Type: "mrkdwn", Text: header}},
	}
}

func (b *Bot) regressionBlocks(event notify.Event) []block {
	header := fmt.Sprintf(":recycle: *Fault Regression* — %s", event.Title)
	return []block{
		{Type: "section", Text: &textObj{Type: "mrkdwn", Text: header}},
	}
}
