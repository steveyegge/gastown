package db

import (
	"context"
	"os"
	"testing"
)

func openTeamsTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_teams_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM team_projects")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM team_members")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM teams")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM projects")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM auth_sessions")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM accounts")
		_ = d.Close()
	})
	return d
}

func TestCreateAndGetTeam(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team, err := d.CreateTeam(ctx, "Backend Team", "backend")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if team.Name != "Backend Team" {
		t.Errorf("expected name 'Backend Team', got %q", team.Name)
	}
	if team.Slug != "backend" {
		t.Errorf("expected slug 'backend', got %q", team.Slug)
	}

	got, err := d.GetTeam(ctx, team.ID)
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if got.Name != "Backend Team" {
		t.Errorf("expected name 'Backend Team', got %q", got.Name)
	}
}

func TestListTeams(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	_, _ = d.CreateTeam(ctx, "Alpha", "alpha")
	_, _ = d.CreateTeam(ctx, "Beta", "beta")

	teams, err := d.ListTeams(ctx)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	if len(teams) < 2 {
		t.Fatalf("expected at least 2 teams, got %d", len(teams))
	}
	// Should be ordered by name.
	if teams[0].Name > teams[1].Name {
		t.Errorf("expected alphabetical order, got %q before %q", teams[0].Name, teams[1].Name)
	}
}

func TestUpdateTeam(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team, _ := d.CreateTeam(ctx, "Old Name", "old-slug")

	if err := d.UpdateTeam(ctx, team.ID, "New Name", "new-slug"); err != nil {
		t.Fatalf("update team: %v", err)
	}

	got, _ := d.GetTeam(ctx, team.ID)
	if got.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", got.Name)
	}
	if got.Slug != "new-slug" {
		t.Errorf("expected 'new-slug', got %q", got.Slug)
	}
}

func TestDeleteTeam(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team, _ := d.CreateTeam(ctx, "Doomed", "doomed")
	acct, _ := d.CreateAccount(ctx, "del@test.com", "Del", "password123", "member")
	_ = d.AddTeamMember(ctx, team.ID, acct.ID, "member")

	if err := d.DeleteTeam(ctx, team.ID); err != nil {
		t.Fatalf("delete team: %v", err)
	}

	_, err := d.GetTeam(ctx, team.ID)
	if err == nil {
		t.Error("expected error getting deleted team")
	}

	members, _ := d.ListTeamMembers(ctx, team.ID)
	if len(members) != 0 {
		t.Errorf("expected 0 members after delete, got %d", len(members))
	}
}

func TestTeamMembers(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team, _ := d.CreateTeam(ctx, "Dev", "dev")
	acct1, _ := d.CreateAccount(ctx, "alice@test.com", "Alice", "password123", "member")
	acct2, _ := d.CreateAccount(ctx, "bob@test.com", "Bob", "password123", "member")

	// Add members.
	if err := d.AddTeamMember(ctx, team.ID, acct1.ID, "admin"); err != nil {
		t.Fatalf("add member 1: %v", err)
	}
	if err := d.AddTeamMember(ctx, team.ID, acct2.ID, "member"); err != nil {
		t.Fatalf("add member 2: %v", err)
	}

	members, err := d.ListTeamMembers(ctx, team.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// Adding same member again should update role (upsert).
	if err := d.AddTeamMember(ctx, team.ID, acct1.ID, "member"); err != nil {
		t.Fatalf("re-add member: %v", err)
	}
	members, _ = d.ListTeamMembers(ctx, team.ID)
	if len(members) != 2 {
		t.Fatalf("expected 2 members after upsert, got %d", len(members))
	}

	// Remove a member.
	if err := d.RemoveTeamMember(ctx, team.ID, acct2.ID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	members, _ = d.ListTeamMembers(ctx, team.ID)
	if len(members) != 1 {
		t.Errorf("expected 1 member after remove, got %d", len(members))
	}
}

func TestTeamProjects(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team, _ := d.CreateTeam(ctx, "Infra", "infra")

	// Seed projects.
	_ = d.EnsureProject(ctx, 100, "Web App", "web-app", "key100")
	_ = d.EnsureProject(ctx, 200, "Mobile", "mobile", "key200")

	// Link projects.
	if err := d.LinkTeamProject(ctx, team.ID, 100); err != nil {
		t.Fatalf("link project 100: %v", err)
	}
	if err := d.LinkTeamProject(ctx, team.ID, 200); err != nil {
		t.Fatalf("link project 200: %v", err)
	}

	projects, err := d.ListTeamProjects(ctx, team.ID)
	if err != nil {
		t.Fatalf("list team projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Linking again should be idempotent.
	if err := d.LinkTeamProject(ctx, team.ID, 100); err != nil {
		t.Fatalf("re-link: %v", err)
	}
	projects, _ = d.ListTeamProjects(ctx, team.ID)
	if len(projects) != 2 {
		t.Errorf("expected 2 projects after re-link, got %d", len(projects))
	}

	// Unlink one.
	if err := d.UnlinkTeamProject(ctx, team.ID, 200); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	projects, _ = d.ListTeamProjects(ctx, team.ID)
	if len(projects) != 1 {
		t.Errorf("expected 1 project after unlink, got %d", len(projects))
	}
}

func TestTeamsForAccount(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team1, _ := d.CreateTeam(ctx, "Frontend", "frontend")
	team2, _ := d.CreateTeam(ctx, "Backend", "backend-tfa")
	acct, _ := d.CreateAccount(ctx, "multi@test.com", "Multi", "password123", "member")

	_ = d.AddTeamMember(ctx, team1.ID, acct.ID, "member")
	_ = d.AddTeamMember(ctx, team2.ID, acct.ID, "admin")

	teams, err := d.TeamsForAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("teams for account: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}

func TestProjectIDsVisibleTo(t *testing.T) {
	d := openTeamsTestDB(t)
	ctx := context.Background()

	team, _ := d.CreateTeam(ctx, "Vis Team", "vis-team")
	_ = d.EnsureProject(ctx, 300, "Visible", "visible", "key300")
	_ = d.EnsureProject(ctx, 400, "Hidden", "hidden", "key400")
	_ = d.LinkTeamProject(ctx, team.ID, 300)

	member, _ := d.CreateAccount(ctx, "member@test.com", "Member", "password123", "member")
	admin, _ := d.CreateAccount(ctx, "admin@test.com", "Admin", "password123", "admin")
	_ = d.AddTeamMember(ctx, team.ID, member.ID, "member")

	// Admin sees everything (nil return).
	ids, err := d.ProjectIDsVisibleTo(ctx, admin.ID, admin.Role)
	if err != nil {
		t.Fatalf("visible to admin: %v", err)
	}
	if ids != nil {
		t.Errorf("expected nil for admin, got %v", ids)
	}

	// Member sees only linked projects.
	ids, err = d.ProjectIDsVisibleTo(ctx, member.ID, member.Role)
	if err != nil {
		t.Fatalf("visible to member: %v", err)
	}
	if len(ids) != 1 || ids[0] != 300 {
		t.Errorf("expected [300], got %v", ids)
	}

	// Member not in any team sees nothing.
	loner, _ := d.CreateAccount(ctx, "loner@test.com", "Loner", "password123", "viewer")
	ids, err = d.ProjectIDsVisibleTo(ctx, loner.ID, loner.Role)
	if err != nil {
		t.Fatalf("visible to loner: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list for loner, got %v", ids)
	}
}
