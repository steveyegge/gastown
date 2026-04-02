package integrations

import (
	"context"

	"github.com/outdoorsea/faultline/internal/db"
)

// DBAdapter wraps *db.DB to implement ConfigProvider.
type DBAdapter struct {
	DB *db.DB
}

// ListEnabledIntegrations implements ConfigProvider.
func (a *DBAdapter) ListEnabledIntegrations(ctx context.Context, projectID int64) ([]Config, error) {
	rows, err := a.DB.ListEnabledIntegrations(ctx, projectID)
	if err != nil {
		return nil, err
	}
	configs := make([]Config, len(rows))
	for i, r := range rows {
		configs[i] = Config{
			ID:              r.ID,
			IntegrationType: r.IntegrationType,
			Enabled:         r.Enabled,
			Config:          r.Config,
		}
	}
	return configs, nil
}
