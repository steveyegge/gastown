package db

import (
	"context"
	"time"
)

// Project represents a row from the projects table.
type Project struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	DSNPublicKey string    `json:"dsn_public_key,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ListProjects returns all projects.
func (d *DB) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, name, slug, dsn_public_key, created_at FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.DSNPublicKey, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetProject returns a single project by ID.
func (d *DB) GetProject(ctx context.Context, projectID int64) (*Project, error) {
	var p Project
	err := d.QueryRowContext(ctx,
		`SELECT id, name, slug, dsn_public_key, created_at FROM projects WHERE id = ?`,
		projectID,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.DSNPublicKey, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
