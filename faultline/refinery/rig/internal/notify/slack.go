// Package notify provides webhook notification support for faultline events.
// Currently supports Slack incoming webhooks with Block Kit formatting.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// EventType identifies the kind of notification event.
type EventType string

const (
	EventNewIssue   EventType = "new_issue"
	EventResolved   EventType = "resolved"
	EventRegression EventType = "regression"
)

// Event contains the data for a notification.
type Event struct {
	Type       EventType
	ProjectID  int64
	GroupID    string
	Title      string
	Culprit    string
	Level      string
	Platform   string
	EventCount int
	BeadID     string
	PrevBeadID string // for regressions
}

// Notifier sends notifications for faultline events.
type Notifier interface {
	Notify(ctx context.Context, event Event)
}

// SlackWebhook sends notifications to a Slack incoming webhook URL.
type SlackWebhook struct {
	webhookURL string
	client     *http.Client
	log        *slog.Logger
	baseURL    string // faultline dashboard URL for links
}

// NewSlackWebhook creates a Slack webhook notifier.
// Returns nil if webhookURL is empty (notifications disabled).
func NewSlackWebhook(webhookURL, baseURL string, log *slog.Logger) *SlackWebhook {
	if webhookURL == "" {
		return nil
	}
	return &SlackWebhook{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
		log:        log,
		baseURL:    baseURL,
	}
}

// Notify sends a formatted Slack message for the given event.
func (s *SlackWebhook) Notify(ctx context.Context, event Event) {
	payload := s.buildPayload(event)

	body, err := json.Marshal(payload)
	if err != nil {
		s.log.Error("slack: marshal payload", "err", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		s.log.Error("slack: create request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("slack: send webhook", "err", err, "event_type", string(event.Type))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.log.Error("slack: webhook response", "status", resp.StatusCode, "event_type", string(event.Type))
		return
	}

	s.log.Debug("slack: notification sent", "event_type", string(event.Type), "group_id", event.GroupID)
}

// slackPayload is the Slack Block Kit message structure.
type slackPayload struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type     string       `json:"type"`
	Text     *slackText   `json:"text,omitempty"`
	Fields   []slackText  `json:"fields,omitempty"`
	Elements []slackBlock `json:"elements,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *SlackWebhook) buildPayload(event Event) slackPayload {
	switch event.Type {
	case EventNewIssue:
		return s.newIssuePayload(event)
	case EventResolved:
		return s.resolvedPayload(event)
	case EventRegression:
		return s.regressionPayload(event)
	default:
		return slackPayload{}
	}
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

func severityName(level string) string {
	switch level {
	case "fatal":
		return "Rupture"
	case "error":
		return "Quake"
	default:
		return "Tremor"
	}
}

func (s *SlackWebhook) newIssuePayload(event Event) slackPayload {
	emoji := severityEmoji(event.Level)
	severity := severityName(event.Level)

	header := fmt.Sprintf("%s *New Fault Detected* — %s", emoji, severity)

	fields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Exception*\n%s", event.Title)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Level*\n%s", event.Level)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Culprit*\n`%s`", event.Culprit)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Events*\n%d", event.EventCount)},
	}
	if event.Platform != "" {
		fields = append(fields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Platform*\n%s", event.Platform)})
	}
	if event.BeadID != "" {
		fields = append(fields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Bead*\n%s", event.BeadID)})
	}

	blocks := []slackBlock{
		{Type: "section", Text: &slackText{Type: "mrkdwn", Text: header}},
		{Type: "section", Fields: fields},
	}

	if s.baseURL != "" {
		issueURL := fmt.Sprintf("%s/api/%d/issues/%s", s.baseURL, event.ProjectID, event.GroupID)
		blocks = append(blocks, slackBlock{
			Type: "context",
			Elements: []slackBlock{
				{Type: "mrkdwn", Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("<%s|View in Faultline>", issueURL)}},
			},
		})
	}

	return slackPayload{Blocks: blocks}
}

func (s *SlackWebhook) resolvedPayload(event Event) slackPayload {
	header := fmt.Sprintf(":white_check_mark: *Fault Resolved* — %s", event.Title)

	fields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Group*\n%s", shortID(event.GroupID))},
	}
	if event.BeadID != "" {
		fields = append(fields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Bead*\n%s", event.BeadID)})
	}

	return slackPayload{
		Blocks: []slackBlock{
			{Type: "section", Text: &slackText{Type: "mrkdwn", Text: header}},
			{Type: "section", Fields: fields},
		},
	}
}

func (s *SlackWebhook) regressionPayload(event Event) slackPayload {
	header := fmt.Sprintf(":recycle: *Fault Regression* — %s", event.Title)

	fields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Group*\n%s", shortID(event.GroupID))},
	}
	if event.PrevBeadID != "" {
		fields = append(fields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Previous Bead*\n%s", event.PrevBeadID)})
	}
	if event.BeadID != "" {
		fields = append(fields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*New Bead*\n%s", event.BeadID)})
	}

	return slackPayload{
		Blocks: []slackBlock{
			{Type: "section", Text: &slackText{Type: "mrkdwn", Text: header}},
			{Type: "section", Fields: fields},
		},
	}
}

func shortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
