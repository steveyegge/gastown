package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStaleBeadsRedirectCheck_NoStaleFiles(t *testing.T) {
	// Create temp town with clean .beads redirect
	townRoot := t.TempDir()
	rigDir := filepath.Join(townRoot, "myrig")
	beadsDir := filepath.Join(rigDir, ".beads")

	// Create rig structure
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create only redirect file (no stale files)
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("../mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Also need a .git to make it look like a rig
	if err := os.MkdirAll(filepath.Join(rigDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewStaleBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestStaleBeadsRedirectCheck_WithStaleFiles(t *testing.T) {
	// Create temp town with stale .beads files
	townRoot := t.TempDir()
	rigDir := filepath.Join(townRoot, "myrig")
	beadsDir := filepath.Join(rigDir, ".beads")

	// Create rig structure
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("../mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create stale files
	staleFiles := []string{"issues.jsonl", "issues.db", "metadata.json"}
	for _, f := range staleFiles {
		if err := os.WriteFile(filepath.Join(beadsDir, f), []byte("stale data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Also need a .git to make it look like a rig
	if err := os.MkdirAll(filepath.Join(rigDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewStaleBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if len(result.Details) != 1 {
		t.Errorf("Expected 1 stale location, got %d", len(result.Details))
	}
}

func TestStaleBeadsRedirectCheck_FixRemovesStaleFiles(t *testing.T) {
	// Create temp town with stale .beads files
	townRoot := t.TempDir()
	rigDir := filepath.Join(townRoot, "myrig")
	beadsDir := filepath.Join(rigDir, ".beads")

	// Create rig structure
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("../mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create stale files (config.yaml excluded - may be tracked in git)
	staleFiles := []string{"issues.jsonl", "issues.db", "metadata.json"}
	for _, f := range staleFiles {
		if err := os.WriteFile(filepath.Join(beadsDir, f), []byte("stale data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create .gitignore (should be preserved)
	gitignorePath := filepath.Join(beadsDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("*.db\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Also need a .git to make it look like a rig
	if err := os.MkdirAll(filepath.Join(rigDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewStaleBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	// Run to detect issues
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify stale files removed
	for _, f := range staleFiles {
		if _, err := os.Stat(filepath.Join(beadsDir, f)); !os.IsNotExist(err) {
			t.Errorf("Stale file %s still exists after fix", f)
		}
	}

	// Verify redirect preserved
	if _, err := os.Stat(redirectPath); err != nil {
		t.Errorf("Redirect file should be preserved: %v", err)
	}

	// Verify .gitignore preserved
	if _, err := os.Stat(gitignorePath); err != nil {
		t.Errorf(".gitignore should be preserved: %v", err)
	}

	// Run again to verify clean
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

func TestStaleBeadsRedirectCheck_NoRedirect(t *testing.T) {
	// Create temp town with .beads but no redirect (canonical location)
	townRoot := t.TempDir()
	rigDir := filepath.Join(townRoot, "myrig")
	beadsDir := filepath.Join(rigDir, ".beads")

	// Create rig structure
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create data files but NO redirect
	if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Also need a .git to make it look like a rig
	if err := os.MkdirAll(filepath.Join(rigDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewStaleBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)

	// Should be OK - no redirect means this is a canonical location
	if result.Status != StatusOK {
		t.Errorf("Expected StatusOK (no redirect), got %v: %s", result.Status, result.Message)
	}
}

func TestStaleBeadsRedirectCheck_CrewWorkspaces(t *testing.T) {
	// Create temp town with crew workspace stale files
	townRoot := t.TempDir()
	rigDir := filepath.Join(townRoot, "myrig")
	crewBeadsDir := filepath.Join(rigDir, "crew", "worker1", ".beads")

	// Create crew structure
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create redirect file
	redirectPath := filepath.Join(crewBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("../../../.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create stale file
	if err := os.WriteFile(filepath.Join(crewBeadsDir, "issues.db"), []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	// Also need a .git to make it look like a rig
	if err := os.MkdirAll(filepath.Join(rigDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewStaleBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Expected StatusWarning for crew stale files, got %v: %s", result.Status, result.Message)
	}
}
