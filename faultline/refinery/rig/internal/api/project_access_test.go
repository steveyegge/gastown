package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/outdoorsea/faultline/internal/db"
)

func openAccessTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_access_test?parseTime=true"
	}
	d, err := db.Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = d.ExecContext(ctx, "DELETE FROM team_projects")
		_, _ = d.ExecContext(ctx, "DELETE FROM team_members")
		_, _ = d.ExecContext(ctx, "DELETE FROM teams")
		_, _ = d.ExecContext(ctx, "DELETE FROM projects")
		_, _ = d.ExecContext(ctx, "DELETE FROM accounts")
		_ = d.Close()
	})
	return d
}

func TestRequireProjectAccess(t *testing.T) {
	d := openAccessTestDB(t)
	ctx := context.Background()

	h := &Handler{
		DB:  d,
		Log: slog.Default(),
	}

	// Seed data: team, project, accounts.
	team, err := d.CreateTeam(ctx, "Access Team", "access-team")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	_ = d.EnsureProject(ctx, 10, "Allowed Project", "allowed", "key10")
	_ = d.EnsureProject(ctx, 20, "Forbidden Project", "forbidden", "key20")
	_ = d.LinkTeamProject(ctx, team.ID, 10)

	member, _ := d.CreateAccount(ctx, "member@access.test", "Member", "pw", "member")
	_ = d.AddTeamMember(ctx, team.ID, member.ID, "member")

	admin, _ := d.CreateAccount(ctx, "admin@access.test", "Admin", "pw", "admin")
	owner, _ := d.CreateAccount(ctx, "owner@access.test", "Owner", "pw", "owner")
	viewer, _ := d.CreateAccount(ctx, "viewer@access.test", "Viewer", "pw", "viewer")

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := h.requireProjectAccess(inner)

	tests := []struct {
		name       string
		projectID  string
		accountID  string
		role       string
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "owner bypasses access check",
			projectID:  "20",
			accountID:  formatInt64(owner.ID),
			role:       "owner",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "admin bypasses access check",
			projectID:  "20",
			accountID:  formatInt64(admin.ID),
			role:       "admin",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "member accesses linked project",
			projectID:  "10",
			accountID:  formatInt64(member.ID),
			role:       "member",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "member denied unlinked project",
			projectID:  "20",
			accountID:  formatInt64(member.ID),
			role:       "member",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "viewer denied (no team membership)",
			projectID:  "10",
			accountID:  formatInt64(viewer.ID),
			role:       "viewer",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest("GET", "/api/"+tt.projectID+"/issues/", nil)
			r.SetPathValue("project_id", tt.projectID)
			r.Header.Set("X-Account-ID", tt.accountID)
			r.Header.Set("X-Account-Role", tt.role)
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestTokenProjectScope(t *testing.T) {
	d := openAccessTestDB(t)
	ctx := context.Background()

	h := &Handler{
		DB:  d,
		Log: slog.Default(),
	}

	_ = d.EnsureProject(ctx, 10, "Project A", "proj-a", "keyA")
	_ = d.EnsureProject(ctx, 20, "Project B", "proj-b", "keyB")
	owner, _ := d.CreateAccount(ctx, "owner@scope.test", "Owner", "pw", "owner")

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := h.requireProjectAccess(inner)

	tests := []struct {
		name           string
		projectID      string
		tokenProjectID string // empty = org-wide token
		role           string
		wantStatus     int
		wantCalled     bool
	}{
		{
			name:           "org-wide token accesses any project",
			projectID:      "10",
			tokenProjectID: "",
			role:           "owner",
			wantStatus:     http.StatusOK,
			wantCalled:     true,
		},
		{
			name:           "scoped token accesses its project",
			projectID:      "10",
			tokenProjectID: "10",
			role:           "owner",
			wantStatus:     http.StatusOK,
			wantCalled:     true,
		},
		{
			name:           "scoped token denied different project",
			projectID:      "20",
			tokenProjectID: "10",
			role:           "owner",
			wantStatus:     http.StatusForbidden,
			wantCalled:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest("GET", "/api/"+tt.projectID+"/issues/", nil)
			r.SetPathValue("project_id", tt.projectID)
			r.Header.Set("X-Account-ID", formatInt64(owner.ID))
			r.Header.Set("X-Account-Role", tt.role)
			if tt.tokenProjectID != "" {
				r.Header.Set("X-Token-Project-ID", tt.tokenProjectID)
			}
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestListProjectsFiltered(t *testing.T) {
	d := openAccessTestDB(t)
	ctx := context.Background()

	h := &Handler{
		DB:  d,
		Log: slog.Default(),
	}

	// Seed data.
	team, _ := d.CreateTeam(ctx, "Filter Team", "filter-team")
	_ = d.EnsureProject(ctx, 100, "Visible", "visible", "key100")
	_ = d.EnsureProject(ctx, 200, "Hidden", "hidden", "key200")
	_ = d.LinkTeamProject(ctx, team.ID, 100)

	member, _ := d.CreateAccount(ctx, "member@filter.test", "Member", "pw", "member")
	_ = d.AddTeamMember(ctx, team.ID, member.ID, "member")
	admin, _ := d.CreateAccount(ctx, "admin@filter.test", "Admin", "pw", "admin")

	// Member should only see project 100.
	r := httptest.NewRequest("GET", "/api/v1/projects/", nil)
	r.Header.Set("X-Account-ID", formatInt64(member.ID))
	r.Header.Set("X-Account-Role", "member")
	w := httptest.NewRecorder()
	h.listProjects(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Projects []db.Project `json:"projects"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Projects) != 1 {
		t.Fatalf("expected 1 project for member, got %d", len(resp.Projects))
	}
	if resp.Projects[0].ID != 100 {
		t.Errorf("expected project 100, got %d", resp.Projects[0].ID)
	}

	// Admin should see all projects.
	r = httptest.NewRequest("GET", "/api/v1/projects/", nil)
	r.Header.Set("X-Account-ID", formatInt64(admin.ID))
	r.Header.Set("X-Account-Role", "admin")
	w = httptest.NewRecorder()
	h.listProjects(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Projects) < 2 {
		t.Errorf("expected at least 2 projects for admin, got %d", len(resp.Projects))
	}
}
