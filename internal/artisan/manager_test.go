package artisan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"frontend-1", false},
		{"backend-2", false},
		{"tests-1", false},
		{"docs-1", false},
		{"security-1", false},
		{"myworker", false},
		{"", true},
		{".", true},
		{"..", true},
		{"a/b", true},
		{"a\\b", true},
		{"a..b", true},
		{"a b", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestManager_Add(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)

	worker, err := mgr.Add("frontend-1", "frontend")
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	if worker.Name != "frontend-1" {
		t.Errorf("Name = %q, want %q", worker.Name, "frontend-1")
	}
	if worker.Specialty != "frontend" {
		t.Errorf("Specialty = %q, want %q", worker.Specialty, "frontend")
	}
	if worker.Rig != "gastown" {
		t.Errorf("Rig = %q, want %q", worker.Rig, "gastown")
	}

	// Directory should exist
	dir := filepath.Join(rigPath, "artisans", "frontend-1")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("artisan directory was not created")
	}

	// Mail directory should exist
	mailDir := filepath.Join(dir, "mail")
	if _, err := os.Stat(mailDir); os.IsNotExist(err) {
		t.Error("mail directory was not created")
	}

	// State file should exist
	stateFile := filepath.Join(dir, "state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("state.json was not created")
	}
}

func TestManager_Add_Duplicate(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)

	_, err := mgr.Add("frontend-1", "frontend")
	if err != nil {
		t.Fatalf("first Add() error: %v", err)
	}

	_, err = mgr.Add("frontend-1", "frontend")
	if err == nil {
		t.Error("second Add() should have returned error")
	}
}

func TestManager_List(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)

	// Empty list
	workers, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(workers) != 0 {
		t.Errorf("List() returned %d workers, want 0", len(workers))
	}

	// Add some artisans
	mgr.Add("frontend-1", "frontend")
	mgr.Add("backend-1", "backend")
	mgr.Add("tests-1", "tests")

	workers, err = mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(workers) != 3 {
		t.Errorf("List() returned %d workers, want 3", len(workers))
	}

	// Verify specialties are preserved
	specialties := make(map[string]string)
	for _, w := range workers {
		specialties[w.Name] = w.Specialty
	}
	if specialties["frontend-1"] != "frontend" {
		t.Errorf("frontend-1 specialty = %q, want %q", specialties["frontend-1"], "frontend")
	}
	if specialties["backend-1"] != "backend" {
		t.Errorf("backend-1 specialty = %q, want %q", specialties["backend-1"], "backend")
	}
}

func TestManager_Get(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)
	mgr.Add("frontend-1", "frontend")

	worker, err := mgr.Get("frontend-1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if worker.Name != "frontend-1" {
		t.Errorf("Name = %q, want %q", worker.Name, "frontend-1")
	}
	if worker.Specialty != "frontend" {
		t.Errorf("Specialty = %q, want %q", worker.Specialty, "frontend")
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Error("Get() should have returned error for nonexistent artisan")
	}
}

func TestManager_Remove(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)
	mgr.Add("frontend-1", "frontend")

	err := mgr.Remove("frontend-1", false)
	if err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	// Should be gone
	dir := filepath.Join(rigPath, "artisans", "frontend-1")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("artisan directory should have been removed")
	}
}

func TestManager_Remove_NotFound(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)

	err := mgr.Remove("nonexistent", false)
	if err == nil {
		t.Error("Remove() should have returned error for nonexistent artisan")
	}
}

func TestManager_MultipleSpecialties(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	os.MkdirAll(rigPath, 0o755)

	mgr := NewManager("gastown", rigPath, townRoot)

	specialties := []struct {
		name      string
		specialty string
	}{
		{"frontend-1", "frontend"},
		{"frontend-2", "frontend"},
		{"backend-1", "backend"},
		{"tests-1", "tests"},
		{"docs-1", "docs"},
		{"security-1", "security"},
	}

	for _, s := range specialties {
		_, err := mgr.Add(s.name, s.specialty)
		if err != nil {
			t.Fatalf("Add(%s, %s) error: %v", s.name, s.specialty, err)
		}
	}

	workers, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(workers) != 6 {
		t.Errorf("List() returned %d workers, want 6", len(workers))
	}

	// Count by specialty
	counts := make(map[string]int)
	for _, w := range workers {
		counts[w.Specialty]++
	}
	if counts["frontend"] != 2 {
		t.Errorf("frontend count = %d, want 2", counts["frontend"])
	}
	if counts["backend"] != 1 {
		t.Errorf("backend count = %d, want 1", counts["backend"])
	}
}
