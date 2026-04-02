package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Notification represents a row from the notifications table.
type Notification struct {
	ID        string    `json:"id"`
	AccountID int64     `json:"account_id"`
	Type      string    `json:"type"`       // mention, assignment, status_change, comment
	IssueID   string    `json:"issue_id"`   // issue_groups.id
	CommentID *string   `json:"comment_id"` // nullable, references issue_comments.id
	ActorID   int64     `json:"actor_id"`   // account that triggered the notification
	ActorName string    `json:"actor_name"` // denormalized for display
	Title     string    `json:"title"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// migrateNotifications creates the notifications table.
func (d *DB) migrateNotifications(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notifications (
		id         VARCHAR(36) PRIMARY KEY,
		account_id BIGINT NOT NULL,
		type       VARCHAR(32) NOT NULL,
		issue_id   VARCHAR(36) NOT NULL,
		comment_id VARCHAR(36),
		actor_id   BIGINT NOT NULL,
		actor_name VARCHAR(256) NOT NULL,
		title      VARCHAR(512) NOT NULL,
		read_at    DATETIME(6),
		created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		INDEX idx_account_read (account_id, read_at),
		INDEX idx_account_created (account_id, created_at)
	)`)
	if err != nil {
		return fmt.Errorf("migrate notifications: %w", err)
	}
	return nil
}

// CreateNotification inserts a new notification.
func (d *DB) CreateNotification(ctx context.Context, accountID int64, nType, issueID string, commentID *string, actorID int64, actorName, title string) (*Notification, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := d.ExecContext(ctx,
		`INSERT INTO notifications (id, account_id, type, issue_id, comment_id, actor_id, actor_name, title, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, accountID, nType, issueID, commentID, actorID, actorName, title, now,
	)
	if err != nil {
		return nil, err
	}
	d.MarkDirty()
	return &Notification{
		ID:        id,
		AccountID: accountID,
		Type:      nType,
		IssueID:   issueID,
		CommentID: commentID,
		ActorID:   actorID,
		ActorName: actorName,
		Title:     title,
		CreatedAt: now,
	}, nil
}

// ListNotifications returns notifications for an account, newest first.
func (d *DB) ListNotifications(ctx context.Context, accountID int64, limit, offset int) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx,
		`SELECT id, account_id, type, issue_id, comment_id, actor_id, actor_name, title, read_at, created_at
		 FROM notifications
		 WHERE account_id = ?
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		accountID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.AccountID, &n.Type, &n.IssueID, &n.CommentID,
			&n.ActorID, &n.ActorName, &n.Title, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadNotificationCount returns the number of unread notifications for an account.
func (d *DB) UnreadNotificationCount(ctx context.Context, accountID int64) (int, error) {
	var count int
	err := d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE account_id = ? AND read_at IS NULL`,
		accountID,
	).Scan(&count)
	return count, err
}

// MarkNotificationsRead marks specific notifications as read.
func (d *DB) MarkNotificationsRead(ctx context.Context, accountID int64, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, time.Now().UTC(), accountID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`UPDATE notifications SET read_at = ? WHERE account_id = ? AND id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := d.ExecContext(ctx, query, args...)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// MarkAllNotificationsRead marks all unread notifications as read for an account.
func (d *DB) MarkAllNotificationsRead(ctx context.Context, accountID int64) error {
	_, err := d.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE account_id = ? AND read_at IS NULL`,
		time.Now().UTC(), accountID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// AccountsByNames returns accounts matching the given names (case-insensitive).
// Used to resolve @mention names to account IDs for notification creation.
func (d *DB) AccountsByNames(ctx context.Context, names []string) ([]Account, error) {
	if len(names) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(names))
	args := make([]interface{}, len(names))
	for i, name := range names {
		placeholders[i] = "?"
		args[i] = name
	}
	query := fmt.Sprintf(
		`SELECT id, email, name, role, created_at FROM accounts WHERE LOWER(name) IN (%s)`,
		strings.Join(placeholders, ","),
	)
	// Lowercase the args for case-insensitive match.
	for i, name := range names {
		args[i] = strings.ToLower(name)
	}
	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
