package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/outdoorsea/faultline/internal/notify"
)

// PagerDuty Events API v2 endpoint.
const pagerDutyEventsURL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyConfig holds per-project configuration for PagerDuty.
type PagerDutyConfig struct {
	RoutingKey      string            `json:"routing_key"`
	ServiceID       string            `json:"service_id,omitempty"`
	SeverityMapping map[string]string `json:"severity_mapping,omitempty"` // faultline level → PD severity
}

// pagerDuty implements the Integration interface for PagerDuty Events API v2.
type pagerDuty struct {
	config PagerDutyConfig
	client *http.Client
}

func init() {
	Register(TypePagerDuty, newPagerDuty)
}

func newPagerDuty(raw json.RawMessage) (Integration, error) {
	var cfg PagerDutyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("pagerduty config: %w", err)
	}
	if cfg.RoutingKey == "" {
		return nil, fmt.Errorf("pagerduty: routing_key is required")
	}
	if cfg.SeverityMapping == nil {
		cfg.SeverityMapping = map[string]string{
			"fatal":   "critical",
			"error":   "error",
			"warning": "warning",
			"info":    "info",
		}
	}
	return &pagerDuty{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *pagerDuty) Type() IntegrationType { return TypePagerDuty }

// OnNewIssue triggers a PagerDuty incident for new issues.
func (p *pagerDuty) OnNewIssue(ctx context.Context, event notify.Event) error {
	return p.trigger(ctx, event)
}

// OnResolved auto-resolves the PagerDuty incident.
func (p *pagerDuty) OnResolved(ctx context.Context, event notify.Event) error {
	return p.resolve(ctx, event)
}

// OnRegression re-triggers a PagerDuty incident for regressions.
func (p *pagerDuty) OnRegression(ctx context.Context, event notify.Event) error {
	return p.trigger(ctx, event)
}

// pdEvent is the PagerDuty Events API v2 request body.
type pdEvent struct {
	RoutingKey  string    `json:"routing_key"`
	EventAction string    `json:"event_action"` // trigger, resolve
	DedupKey    string    `json:"dedup_key"`
	Payload     *pdPaylod `json:"payload,omitempty"`
}

type pdPaylod struct {
	Summary       string            `json:"summary"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"`
	Timestamp     string            `json:"timestamp,omitempty"`
	Component     string            `json:"component,omitempty"`
	Group         string            `json:"group,omitempty"`
	Class         string            `json:"class,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

func (p *pagerDuty) trigger(ctx context.Context, event notify.Event) error {
	severity := p.mapSeverity(event.Level)
	summary := fmt.Sprintf("[%s] %s", event.Level, event.Title)
	if event.Culprit != "" {
		summary = fmt.Sprintf("[%s] %s in %s", event.Level, event.Title, event.Culprit)
	}

	details := map[string]string{
		"group_id":    event.GroupID,
		"platform":    event.Platform,
		"event_count": fmt.Sprintf("%d", event.EventCount),
	}

	ev := pdEvent{
		RoutingKey:  p.config.RoutingKey,
		EventAction: "trigger",
		DedupKey:    p.dedupKey(event),
		Payload: &pdPaylod{
			Summary:       summary,
			Source:        "faultline",
			Severity:      severity,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Component:     p.config.ServiceID,
			Group:         event.Platform,
			Class:         string(event.Type),
			CustomDetails: details,
		},
	}
	return p.send(ctx, ev)
}

func (p *pagerDuty) resolve(ctx context.Context, event notify.Event) error {
	ev := pdEvent{
		RoutingKey:  p.config.RoutingKey,
		EventAction: "resolve",
		DedupKey:    p.dedupKey(event),
	}
	return p.send(ctx, ev)
}

func (p *pagerDuty) dedupKey(event notify.Event) string {
	return fmt.Sprintf("faultline-%d-%s", event.ProjectID, event.GroupID)
}

func (p *pagerDuty) mapSeverity(level string) string {
	if s, ok := p.config.SeverityMapping[level]; ok {
		return s
	}
	return "error"
}

func (p *pagerDuty) send(ctx context.Context, ev pdEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("pagerduty: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pagerDutyEventsURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pagerduty: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pagerduty: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pagerduty: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
