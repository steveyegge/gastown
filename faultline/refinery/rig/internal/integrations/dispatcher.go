package integrations

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/outdoorsea/faultline/internal/notify"
)

// ConfigProvider loads enabled integration configs for a project.
type ConfigProvider interface {
	ListEnabledIntegrations(ctx context.Context, projectID int64) ([]Config, error)
}

// Config mirrors db.IntegrationConfig but avoids a circular import.
type Config struct {
	ID              string
	IntegrationType string
	Enabled         bool
	Config          json.RawMessage
}

// Dispatcher loads integration configs from the database and dispatches
// events to all enabled integrations for a project. It implements
// notify.IntegrationNotifier.
type Dispatcher struct {
	provider ConfigProvider
	log      *slog.Logger
}

// NewDispatcher creates an integration dispatcher.
func NewDispatcher(provider ConfigProvider, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		provider: provider,
		log:      log,
	}
}

// NotifyIntegrations fires all enabled integrations for the event's project.
func (d *Dispatcher) NotifyIntegrations(ctx context.Context, event notify.Event) {
	configs, err := d.provider.ListEnabledIntegrations(ctx, event.ProjectID)
	if err != nil {
		d.log.Error("load integrations", "project_id", event.ProjectID, "err", err)
		return
	}

	for _, cfg := range configs {
		intg, err := New(IntegrationType(cfg.IntegrationType), cfg.Config)
		if err != nil {
			d.log.Warn("skip integration", "type", cfg.IntegrationType, "id", cfg.ID, "err", err)
			continue
		}
		if err := Dispatch(ctx, intg, event); err != nil {
			d.log.Error("integration dispatch", "type", cfg.IntegrationType, "id", cfg.ID, "event", event.Type, "err", err)
		}
	}
}
