package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SlackUserMapping links a faultline account to a Slack user.
type SlackUserMapping struct {
	ID          int64     `json:"id"`
	AccountID   int64     `json:"account_id"`
	SlackUserID string    `json:"slack_user_id"`
	SlackTeamID string    `json:"slack_team_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// migrateSlackUserMappings creates the slack_user_mappings table.
func (d *DB) migrateSlackUserMappings(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS slack_user_mappings (
		id            BIGINT AUTO_INCREMENT PRIMARY KEY,
		account_id    BIGINT NOT NULL,
		slack_user_id VARCHAR(64) NOT NULL,
		slack_team_id VARCHAR(64) NOT NULL DEFAULT '',
		created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		UNIQUE KEY uq_slack_account (account_id),
		UNIQUE KEY uq_slack_user (slack_user_id, slack_team_id),
		INDEX idx_slack_account (account_id)
	)`)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("migrate slack_user_mappings: %w", err)
		}
	}
	return nil
}

// UpsertSlackUserMapping links a faultline account to a Slack user.
// If the account already has a mapping, it is replaced.
func (d *DB) UpsertSlackUserMapping(ctx context.Context, accountID int64, slackUserID, slackTeamID string) error {
	_, err := d.ExecContext(ctx, `
		REPLACE INTO slack_user_mappings (account_id, slack_user_id, slack_team_id, created_at)
		VALUES (?, ?, ?, ?)`,
		accountID, slackUserID, slackTeamID, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert slack mapping: %w", err)
	}
	d.MarkDirty()
	return nil
}

// DeleteSlackUserMapping removes a Slack link for an account.
func (d *DB) DeleteSlackUserMapping(ctx context.Context, accountID int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM slack_user_mappings WHERE account_id = ?`, accountID)
	if err != nil {
		return fmt.Errorf("delete slack mapping: %w", err)
	}
	d.MarkDirty()
	return nil
}

// GetSlackUserMapping returns the Slack mapping for an account, or nil if none.
func (d *DB) GetSlackUserMapping(ctx context.Context, accountID int64) (*SlackUserMapping, error) {
	var m SlackUserMapping
	err := d.QueryRowContext(ctx,
		`SELECT id, account_id, slack_user_id, slack_team_id, created_at
		 FROM slack_user_mappings WHERE account_id = ?`, accountID,
	).Scan(&m.ID, &m.AccountID, &m.SlackUserID, &m.SlackTeamID, &m.CreatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// SlackUserIDForAccount returns the Slack user ID for a given account, or empty string.
func (d *DB) SlackUserIDForAccount(ctx context.Context, accountID int64) (string, error) {
	var slackUserID string
	err := d.QueryRowContext(ctx,
		`SELECT slack_user_id FROM slack_user_mappings WHERE account_id = ?`, accountID,
	).Scan(&slackUserID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return "", nil
		}
		return "", err
	}
	return slackUserID, nil
}

// ListSlackUserMappings returns all Slack user mappings.
func (d *DB) ListSlackUserMappings(ctx context.Context) ([]SlackUserMapping, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, account_id, slack_user_id, slack_team_id, created_at
		 FROM slack_user_mappings ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []SlackUserMapping
	for rows.Next() {
		var m SlackUserMapping
		if err := rows.Scan(&m.ID, &m.AccountID, &m.SlackUserID, &m.SlackTeamID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
