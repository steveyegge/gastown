package followup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndList(t *testing.T) {
	townRoot := t.TempDir()
	// Create .runtime dir
	os.MkdirAll(filepath.Join(townRoot, ".runtime"), 0755)

	agent := "gastown/polecats/Toast"
	due := time.Now().Add(30 * time.Minute)

	f, err := Create(townRoot, agent, "Check PR feedback from Rome", due, "gt-abc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if f.Status != StatusPending {
		t.Fatalf("expected pending, got %s", f.Status)
	}
	if f.BeadID != "gt-abc" {
		t.Fatalf("expected bead gt-abc, got %s", f.BeadID)
	}

	all, err := List(townRoot, agent)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 followup, got %d", len(all))
	}
	if all[0].ID != f.ID {
		t.Fatalf("expected ID %s, got %s", f.ID, all[0].ID)
	}
}

func TestResolve(t *testing.T) {
	townRoot := t.TempDir()
	os.MkdirAll(filepath.Join(townRoot, ".runtime"), 0755)

	agent := "gastown/polecats/Toast"
	f, err := Create(townRoot, agent, "Check feedback", time.Now().Add(10*time.Minute), "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Resolve(townRoot, agent, f.ID); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	all, err := List(townRoot, agent)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}
	if all[0].Status != StatusResolved {
		t.Fatalf("expected resolved, got %s", all[0].Status)
	}
	if all[0].ResolvedAt == nil {
		t.Fatal("expected ResolvedAt to be set")
	}
}

func TestListPending_FiltersResolved(t *testing.T) {
	townRoot := t.TempDir()
	os.MkdirAll(filepath.Join(townRoot, ".runtime"), 0755)

	agent := "gastown/polecats/Toast"
	f1, _ := Create(townRoot, agent, "First", time.Now().Add(10*time.Minute), "")
	Create(townRoot, agent, "Second", time.Now().Add(20*time.Minute), "")
	Resolve(townRoot, agent, f1.ID)

	pending, err := ListPending(townRoot, agent)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Topic != "Second" {
		t.Fatalf("expected 'Second', got %q", pending[0].Topic)
	}
}

func TestListAllPending_AcrossAgents(t *testing.T) {
	townRoot := t.TempDir()
	os.MkdirAll(filepath.Join(townRoot, ".runtime"), 0755)

	Create(townRoot, "rig/polecats/A", "Task A", time.Now().Add(10*time.Minute), "")
	Create(townRoot, "rig/polecats/B", "Task B", time.Now().Add(20*time.Minute), "")

	all, err := ListAllPending(townRoot)
	if err != nil {
		t.Fatalf("ListAllPending: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestIsOverdue(t *testing.T) {
	f := &Followup{
		DueAt:  time.Now().Add(-5 * time.Minute),
		Status: StatusPending,
	}
	if !f.IsOverdue() {
		t.Fatal("expected overdue")
	}

	f.DueAt = time.Now().Add(5 * time.Minute)
	if f.IsOverdue() {
		t.Fatal("expected not overdue")
	}

	f.DueAt = time.Now().Add(-5 * time.Minute)
	f.Status = StatusResolved
	if f.IsOverdue() {
		t.Fatal("resolved should not be overdue")
	}
}

func TestListEmpty(t *testing.T) {
	townRoot := t.TempDir()
	agent := "nonexistent/agent"

	all, err := List(townRoot, agent)
	if err != nil {
		t.Fatalf("List on empty: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0, got %d", len(all))
	}
}

func TestResolveNotFound(t *testing.T) {
	townRoot := t.TempDir()
	os.MkdirAll(filepath.Join(townRoot, ".runtime"), 0755)

	err := Resolve(townRoot, "some/agent", "abcdef0123456789")
	if err == nil {
		t.Fatal("expected error for nonexistent followup")
	}
}

func TestResolveRejectsPathTraversal(t *testing.T) {
	townRoot := t.TempDir()

	err := Resolve(townRoot, "some/agent", "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal ID")
	}
	if !strings.Contains(err.Error(), "invalid followup ID") {
		t.Fatalf("expected validation error, got: %v", err)
	}
}

func TestResolveRejectsNonHexID(t *testing.T) {
	townRoot := t.TempDir()

	for _, bad := range []string{"", "hello-world", "abc/def", "abc..def"} {
		err := Resolve(townRoot, "some/agent", bad)
		if err == nil {
			t.Fatalf("expected error for ID %q", bad)
		}
	}
}

func TestValidateID(t *testing.T) {
	// Valid hex IDs
	for _, good := range []string{"abcdef01", "0123456789abcdef"} {
		if err := validateID(good); err != nil {
			t.Fatalf("expected valid for %q: %v", good, err)
		}
	}
}
