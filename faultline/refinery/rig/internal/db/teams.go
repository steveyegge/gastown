package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Team represents a row from the teams table.
type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamMember represents a row from the team_members table.
type TeamMember struct {
	TeamID    int64     `json:"team_id"`
	AccountID int64     `json:"account_id"`
	Role      string    `json:"role"` // admin, member
	JoinedAt  time.Time `json:"joined_at"`
}

// TeamProject represents a row from the team_projects table.
type TeamProject struct {
	TeamID    int64     `json:"team_id"`
	ProjectID int64     `json:"project_id"`
	LinkedAt  time.Time `json:"linked_at"`
}

// migrateTeams creates the teams, team_members, and team_projects tables.
func (d *DB) migrateTeams(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS teams (
			id         BIGINT AUTO_INCREMENT PRIMARY KEY,
			name       VARCHAR(200) NOT NULL,
			slug       VARCHAR(100) UNIQUE NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
		)`,
		`CREATE TABLE IF NOT EXISTS team_members (
			team_id    BIGINT NOT NULL,
			account_id BIGINT NOT NULL,
			role       VARCHAR(16) NOT NULL DEFAULT 'member',
			joined_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			PRIMARY KEY (team_id, account_id),
			INDEX idx_account (account_id)
		)`,
		`CREATE TABLE IF NOT EXISTS team_projects (
			team_id    BIGINT NOT NULL,
			project_id BIGINT NOT NULL,
			linked_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			PRIMARY KEY (team_id, project_id),
			INDEX idx_project (project_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate teams: %w", err)
		}
	}
	return nil
}

// CreateTeam inserts a new team.
func (d *DB) CreateTeam(ctx context.Context, name, slug string) (*Team, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO teams (name, slug) VALUES (?, ?)`, name, slug,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	d.MarkDirty()
	return &Team{ID: id, Name: name, Slug: slug, CreatedAt: time.Now().UTC()}, nil
}

// GetTeam returns a single team by ID.
func (d *DB) GetTeam(ctx context.Context, teamID int64) (*Team, error) {
	var t Team
	err := d.QueryRowContext(ctx,
		`SELECT id, name, slug, created_at FROM teams WHERE id = ?`, teamID,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTeams returns all teams ordered by name.
func (d *DB) ListTeams(ctx context.Context) ([]Team, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, name, slug, created_at FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var teams []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

// UpdateTeam updates a team's name and slug.
func (d *DB) UpdateTeam(ctx context.Context, teamID int64, name, slug string) error {
	sets := []string{}
	args := []interface{}{}
	if name != "" {
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if slug != "" {
		sets = append(sets, "slug = ?")
		args = append(args, slug)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, teamID)
	_, err := d.ExecContext(ctx,
		fmt.Sprintf("UPDATE teams SET %s WHERE id = ?", strings.Join(sets, ", ")), args...,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// DeleteTeam removes a team and its member/project associations.
func (d *DB) DeleteTeam(ctx context.Context, teamID int64) error {
	for _, q := range []string{
		`DELETE FROM team_members WHERE team_id = ?`,
		`DELETE FROM team_projects WHERE team_id = ?`,
		`DELETE FROM teams WHERE id = ?`,
	} {
		if _, err := d.ExecContext(ctx, q, teamID); err != nil {
			return err
		}
	}
	d.MarkDirty()
	return nil
}

// AddTeamMember adds an account to a team with the given role.
func (d *DB) AddTeamMember(ctx context.Context, teamID, accountID int64, role string) error {
	if role == "" {
		role = "member"
	}
	_, err := d.ExecContext(ctx,
		`INSERT INTO team_members (team_id, account_id, role) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE role = VALUES(role)`,
		teamID, accountID, role,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// RemoveTeamMember removes an account from a team.
func (d *DB) RemoveTeamMember(ctx context.Context, teamID, accountID int64) error {
	_, err := d.ExecContext(ctx,
		`DELETE FROM team_members WHERE team_id = ? AND account_id = ?`,
		teamID, accountID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// ListTeamMembers returns all members of a team.
func (d *DB) ListTeamMembers(ctx context.Context, teamID int64) ([]TeamMember, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT team_id, account_id, role, joined_at FROM team_members WHERE team_id = ? ORDER BY joined_at`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var members []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.TeamID, &m.AccountID, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// LinkTeamProject associates a project with a team.
func (d *DB) LinkTeamProject(ctx context.Context, teamID, projectID int64) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO team_projects (team_id, project_id) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE linked_at = linked_at`,
		teamID, projectID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// UnlinkTeamProject removes the association between a project and a team.
func (d *DB) UnlinkTeamProject(ctx context.Context, teamID, projectID int64) error {
	_, err := d.ExecContext(ctx,
		`DELETE FROM team_projects WHERE team_id = ? AND project_id = ?`,
		teamID, projectID,
	)
	if err == nil {
		d.MarkDirty()
	}
	return err
}

// ListTeamProjects returns all project IDs linked to a team.
func (d *DB) ListTeamProjects(ctx context.Context, teamID int64) ([]TeamProject, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT team_id, project_id, linked_at FROM team_projects WHERE team_id = ? ORDER BY linked_at`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var projects []TeamProject
	for rows.Next() {
		var tp TeamProject
		if err := rows.Scan(&tp.TeamID, &tp.ProjectID, &tp.LinkedAt); err != nil {
			return nil, err
		}
		projects = append(projects, tp)
	}
	return projects, rows.Err()
}

// TeamsForAccount returns all teams that an account belongs to.
func (d *DB) TeamsForAccount(ctx context.Context, accountID int64) ([]Team, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT t.id, t.name, t.slug, t.created_at
		 FROM teams t
		 JOIN team_members tm ON t.id = tm.team_id
		 WHERE tm.account_id = ?
		 ORDER BY t.name`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var teams []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

// ProjectIDsVisibleTo returns the set of project IDs that an account can see
// through their team memberships. If the account is an owner or admin,
// this returns nil (meaning all projects are visible).
func (d *DB) ProjectIDsVisibleTo(ctx context.Context, accountID int64, accountRole string) ([]int64, error) {
	// Owners and admins see everything.
	if accountRole == "owner" || accountRole == "admin" {
		return nil, nil
	}

	rows, err := d.QueryContext(ctx,
		`SELECT DISTINCT tp.project_id
		 FROM team_projects tp
		 JOIN team_members tm ON tp.team_id = tm.team_id
		 WHERE tm.account_id = ?
		 ORDER BY tp.project_id`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
