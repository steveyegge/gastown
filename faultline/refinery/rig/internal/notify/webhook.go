package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
)

// GenericPayload is the JSON body sent to generic (non-Slack) webhook URLs.
type GenericPayload struct {
	EventType  string `json:"event_type"`
	ProjectID  int64  `json:"project_id"`
	GroupID    string `json:"group_id"`
	Title      string `json:"title"`
	Culprit    string `json:"culprit,omitempty"`
	Level      string `json:"level,omitempty"`
	Platform   string `json:"platform,omitempty"`
	EventCount int    `json:"event_count,omitempty"`
	BeadID     string `json:"bead_id,omitempty"`
	PrevBeadID string `json:"prev_bead_id,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// GenericWebhook sends a JSON payload to an arbitrary webhook URL.
type GenericWebhook struct {
	webhookURL string
	templates  []db.WebhookTemplate // custom templates; if empty, uses default payload
	baseURL    string
	client     *http.Client
	log        *slog.Logger
}

// NewGenericWebhook creates a generic webhook notifier.
// Returns nil if webhookURL is empty.
func NewGenericWebhook(webhookURL string, log *slog.Logger) *GenericWebhook {
	if webhookURL == "" {
		return nil
	}
	return &GenericWebhook{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
		log:        log,
	}
}

// NewTemplatedWebhook creates a generic webhook notifier that uses custom templates.
// Returns nil if webhookURL is empty.
func NewTemplatedWebhook(webhookURL, baseURL string, templates []db.WebhookTemplate, log *slog.Logger) *GenericWebhook {
	if webhookURL == "" {
		return nil
	}
	return &GenericWebhook{
		webhookURL: webhookURL,
		templates:  templates,
		baseURL:    baseURL,
		client:     &http.Client{Timeout: 10 * time.Second},
		log:        log,
	}
}

// Notify sends a JSON payload for the given event.
func (g *GenericWebhook) Notify(ctx context.Context, event Event) {
	var body []byte

	if tmpl := FindTemplate(g.templates, string(event.Type)); tmpl != nil {
		vars := TemplateVars(event, g.baseURL)
		rendered := RenderTemplate(tmpl.Body, vars)
		body = []byte(rendered)
	} else {
		payload := GenericPayload{
			EventType:  string(event.Type),
			ProjectID:  event.ProjectID,
			GroupID:    event.GroupID,
			Title:      event.Title,
			Culprit:    event.Culprit,
			Level:      event.Level,
			Platform:   event.Platform,
			EventCount: event.EventCount,
			BeadID:     event.BeadID,
			PrevBeadID: event.PrevBeadID,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		}
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			g.log.Error("webhook: marshal payload", "err", err)
			return
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.webhookURL, bytes.NewReader(body))
	if err != nil {
		g.log.Error("webhook: create request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		g.log.Error("webhook: send", "err", err, "event_type", string(event.Type))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		g.log.Error("webhook: response", "status", resp.StatusCode, "event_type", string(event.Type))
		return
	}

	g.log.Debug("webhook: notification sent", "event_type", string(event.Type), "group_id", event.GroupID)
}
