package db

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Comment represents a row from the issue_comments table.
type Comment struct {
	ID        string    `json:"id"`
	ProjectID int64     `json:"project_id"`
	GroupID   string    `json:"group_id"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	Mentions  []string  `json:"mentions,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// mentionRe matches @Name patterns — a leading @ followed by one or more
// word characters (letters, digits, underscores), optionally containing
// dots or hyphens in the middle (e.g., @Jane, @john.doe, @agent-42).
var mentionRe = regexp.MustCompile(`@([\w][\w.\-]*)`)

// ParseMentions extracts unique @Name mentions from text.
func ParseMentions(text string) []string {
	matches := mentionRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	var result []string
	for _, m := range matches {
		name := m[1]
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}

// migrateComments creates the issue_comments table.
func (d *DB) migrateComments(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS issue_comments (
		id          VARCHAR(36) PRIMARY KEY,
		project_id  BIGINT NOT NULL,
		group_id    VARCHAR(36) NOT NULL,
		author_id   BIGINT NOT NULL,
		body        TEXT NOT NULL,
		mentions    TEXT,
		created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		INDEX idx_comments_group (project_id, group_id),
		INDEX idx_comments_author (author_id),
		INDEX idx_comments_created (created_at)
	)`)
	return err
}

// CreateComment inserts a new comment on an issue group.
// Mentions are automatically extracted from the body.
func (d *DB) CreateComment(ctx context.Context, projectID int64, groupID string, authorID int64, body string) (*Comment, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	mentions := ParseMentions(body)
	var mentionsStr *string
	if len(mentions) > 0 {
		s := strings.Join(mentions, ",")
		mentionsStr = &s
	}

	_, err := d.ExecContext(ctx, `
		INSERT INTO issue_comments (id, project_id, group_id, author_id, body, mentions, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, projectID, groupID, authorID, body, mentionsStr, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}
	d.MarkDirty()

	return &Comment{
		ID:        id,
		ProjectID: projectID,
		GroupID:   groupID,
		AuthorID:  authorID,
		Body:      body,
		Mentions:  mentions,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ListComments returns comments for an issue group, oldest first.
func (d *DB) ListComments(ctx context.Context, projectID int64, groupID string, limit, offset int) ([]Comment, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := d.QueryContext(ctx, `
		SELECT id, project_id, group_id, author_id, body, COALESCE(mentions, ''), created_at, updated_at
		FROM issue_comments
		WHERE project_id = ? AND group_id = ?
		ORDER BY created_at ASC
		LIMIT ? OFFSET ?`,
		projectID, groupID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var comments []Comment
	for rows.Next() {
		var c Comment
		var mentionsStr string
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.GroupID, &c.AuthorID, &c.Body, &mentionsStr, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		if mentionsStr != "" {
			c.Mentions = strings.Split(mentionsStr, ",")
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// UpdateComment updates the body of an existing comment.
// Only the original author may update their comment (enforced by the WHERE clause).
// Mentions are re-extracted from the new body.
func (d *DB) UpdateComment(ctx context.Context, commentID string, authorID int64, newBody string) error {
	mentions := ParseMentions(newBody)
	var mentionsStr *string
	if len(mentions) > 0 {
		s := strings.Join(mentions, ",")
		mentionsStr = &s
	}

	res, err := d.ExecContext(ctx, `
		UPDATE issue_comments
		SET body = ?, mentions = ?, updated_at = ?
		WHERE id = ? AND author_id = ?`,
		newBody, mentionsStr, time.Now().UTC(), commentID, authorID,
	)
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("comment not found or not owned by author")
	}
	d.MarkDirty()
	return nil
}

// DeleteComment removes a comment. Only the original author may delete.
func (d *DB) DeleteComment(ctx context.Context, commentID string, authorID int64) error {
	res, err := d.ExecContext(ctx, `
		DELETE FROM issue_comments WHERE id = ? AND author_id = ?`,
		commentID, authorID,
	)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("comment not found or not owned by author")
	}
	d.MarkDirty()
	return nil
}
