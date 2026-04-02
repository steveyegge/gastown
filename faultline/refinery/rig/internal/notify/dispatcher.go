package notify

import (
	"context"
	"log/slog"
	"strings"

	"github.com/outdoorsea/faultline/internal/db"
)

// ProjectConfigProvider looks up webhook configuration for a project.
type ProjectConfigProvider interface {
	WebhookConfig(ctx context.Context, projectID int64) (url, typ string)
	WebhookTemplates(ctx context.Context, projectID int64) []db.WebhookTemplate
}

// IntegrationNotifier fires integration plugins for events.
type IntegrationNotifier interface {
	NotifyIntegrations(ctx context.Context, event Event)
}

// Dispatcher routes notifications to the correct notifier based on project
// configuration. It supports a global fallback (e.g. from env var) plus
// per-project webhook URLs stored in ProjectConfig, and fires registered
// integrations in parallel with webhooks.
type Dispatcher struct {
	fallback     Notifier // global notifier (from FAULTLINE_SLACK_WEBHOOK env)
	provider     ProjectConfigProvider
	integrations IntegrationNotifier // optional integration dispatcher
	baseURL      string
	log          *slog.Logger
}

// NewDispatcher creates a notification dispatcher.
// fallback may be nil (no global webhook configured).
func NewDispatcher(fallback Notifier, provider ProjectConfigProvider, baseURL string, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		fallback: fallback,
		provider: provider,
		baseURL:  baseURL,
		log:      log,
	}
}

// SetIntegrations configures the integration notifier that fires alongside webhooks.
func (d *Dispatcher) SetIntegrations(n IntegrationNotifier) {
	d.integrations = n
}

// Notify sends a notification using the project-level webhook if configured,
// otherwise falls back to the global webhook. If neither is configured, it's a no-op.
// Integrations are always fired regardless of webhook configuration.
func (d *Dispatcher) Notify(ctx context.Context, event Event) {
	// Fire integrations alongside webhooks.
	if d.integrations != nil {
		d.integrations.NotifyIntegrations(ctx, event)
	}

	// Try project-level webhook first.
	if d.provider != nil {
		url, typ := d.provider.WebhookConfig(ctx, event.ProjectID)
		if url != "" {
			templates := d.provider.WebhookTemplates(ctx, event.ProjectID)
			n := d.notifierFor(url, typ, templates)
			if n != nil {
				n.Notify(ctx, event)
				return
			}
		}
	}

	// Fall back to global webhook.
	if d.fallback != nil {
		d.fallback.Notify(ctx, event)
	}
}

func (d *Dispatcher) notifierFor(url, typ string, templates []db.WebhookTemplate) Notifier {
	switch strings.ToLower(typ) {
	case "slack":
		return NewSlackWebhook(url, d.baseURL, d.log)
	case "discord":
		// Discord supports Slack-compatible webhooks via /slack suffix.
		if !strings.HasSuffix(url, "/slack") {
			url = strings.TrimRight(url, "/") + "/slack"
		}
		return NewSlackWebhook(url, d.baseURL, d.log)
	case "generic", "":
		if len(templates) > 0 {
			return NewTemplatedWebhook(url, d.baseURL, templates, d.log)
		}
		return NewGenericWebhook(url, d.log)
	default:
		if len(templates) > 0 {
			return NewTemplatedWebhook(url, d.baseURL, templates, d.log)
		}
		return NewGenericWebhook(url, d.log)
	}
}
