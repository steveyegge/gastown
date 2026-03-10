package conductor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/artisan"
)

func setupTestRig(t *testing.T, workers []struct{ name, specialty string }) *artisan.Manager {
	t.Helper()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	os.MkdirAll(rigPath, 0o755)

	mgr := artisan.NewManager("testrig", rigPath, townRoot)
	for _, w := range workers {
		_, err := mgr.Add(w.name, w.specialty)
		if err != nil {
			t.Fatalf("Add(%s, %s) error: %v", w.name, w.specialty, err)
		}
	}
	return mgr
}

func TestRouter_Route_AllSpecialties(t *testing.T) {
	mgr := setupTestRig(t, []struct{ name, specialty string }{
		{"frontend-1", "frontend"},
		{"backend-1", "backend"},
		{"tests-1", "tests"},
		{"security-1", "security"},
		{"docs-1", "docs"},
	})

	router := NewRouter(mgr)

	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "testrig",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}
	plan, _ := GeneratePlan(input)

	results, err := router.Route(plan.SubBeads)
	if err != nil {
		t.Fatalf("Route() error: %v", err)
	}

	if len(results) != len(plan.SubBeads) {
		t.Errorf("Route() returned %d results, want %d", len(results), len(plan.SubBeads))
	}

	// Each result should have an artisan assigned
	for _, r := range results {
		if r.ArtisanName == "" {
			t.Errorf("sub-bead %q has no artisan assigned", r.SubBead.Title)
		}
	}
}

func TestRouter_Route_MissingSpecialty(t *testing.T) {
	// Only backend artisan, but plan needs frontend too
	mgr := setupTestRig(t, []struct{ name, specialty string }{
		{"backend-1", "backend"},
		{"tests-1", "tests"},
		{"security-1", "security"},
		{"docs-1", "docs"},
	})

	router := NewRouter(mgr)

	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "testrig",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}
	plan, _ := GeneratePlan(input)

	_, err := router.Route(plan.SubBeads)
	if err == nil {
		t.Fatal("Route() should error when specialty is missing")
	}

	urErr, ok := err.(*UnroutableError)
	if !ok {
		t.Fatalf("expected *UnroutableError, got %T", err)
	}
	if len(urErr.Items) == 0 {
		t.Error("UnroutableError should have items")
	}
}

func TestRouter_Route_LoadBalancing(t *testing.T) {
	mgr := setupTestRig(t, []struct{ name, specialty string }{
		{"backend-1", "backend"},
		{"backend-2", "backend"},
		{"tests-1", "tests"},
		{"security-1", "security"},
		{"docs-1", "docs"},
	})

	router := NewRouter(mgr)

	// Create sub-beads that need 2 backend assignments
	subBeads := []SubBead{
		{Specialty: "backend", Title: "backend work 1"},
		{Specialty: "backend", Title: "backend work 2"},
		{Specialty: "tests", Title: "test work"},
	}

	results, err := router.Route(subBeads)
	if err != nil {
		t.Fatalf("Route() error: %v", err)
	}

	// The two backend sub-beads should go to different artisans
	backendAssignees := make(map[string]bool)
	for _, r := range results {
		if r.SubBead.Specialty == "backend" {
			backendAssignees[r.ArtisanName] = true
		}
	}
	if len(backendAssignees) != 2 {
		t.Errorf("expected 2 different backend artisans, got %d", len(backendAssignees))
	}
}

func TestRouter_Route_EmptyWorkers(t *testing.T) {
	mgr := setupTestRig(t, nil)
	router := NewRouter(mgr)

	subBeads := []SubBead{
		{Specialty: "backend", Title: "work"},
	}

	_, err := router.Route(subBeads)
	if err == nil {
		t.Error("Route() should error with no artisans")
	}
}

func TestRouter_Route_EmptySubBeads(t *testing.T) {
	mgr := setupTestRig(t, []struct{ name, specialty string }{
		{"backend-1", "backend"},
	})
	router := NewRouter(mgr)

	results, err := router.Route(nil)
	if err != nil {
		t.Fatalf("Route() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Route(nil) returned %d results, want 0", len(results))
	}
}
