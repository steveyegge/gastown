package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/outdoorsea/faultline/internal/db"
)

// DBProvider implements ProjectConfigProvider by querying the projects table.
type DBProvider struct {
	db  querier
	log *slog.Logger
}

// querier is the subset of *sql.DB / *db.DB needed for webhook config lookup.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// NewDBProvider creates a provider that reads webhook config from the database.
func NewDBProvider(db querier, log *slog.Logger) *DBProvider {
	return &DBProvider{db: db, log: log}
}

// WebhookConfig returns the webhook URL and type for a project.
func (p *DBProvider) WebhookConfig(ctx context.Context, projectID int64) (url, typ string) {
	var cfgJSON sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT config FROM projects WHERE id = ?`, projectID,
	).Scan(&cfgJSON)
	if err != nil || !cfgJSON.Valid || cfgJSON.String == "" {
		return "", ""
	}

	var cfg struct {
		WebhookURL  string `json:"webhook_url"`
		WebhookType string `json:"webhook_type"`
	}
	if json.Unmarshal([]byte(cfgJSON.String), &cfg) != nil {
		return "", ""
	}
	return cfg.WebhookURL, cfg.WebhookType
}

// WebhookTemplates returns the custom webhook templates for a project.
func (p *DBProvider) WebhookTemplates(ctx context.Context, projectID int64) []db.WebhookTemplate {
	var cfgJSON sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT config FROM projects WHERE id = ?`, projectID,
	).Scan(&cfgJSON)
	if err != nil || !cfgJSON.Valid || cfgJSON.String == "" {
		return nil
	}

	var cfg struct {
		WebhookTemplates []db.WebhookTemplate `json:"webhook_templates"`
	}
	if json.Unmarshal([]byte(cfgJSON.String), &cfg) != nil {
		return nil
	}
	return cfg.WebhookTemplates
}
