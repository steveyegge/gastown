package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLandWorktreeGitignoreCheck_Present(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := createRigWithGitignore(t, townRoot, "myrig", ".land-worktree/\n")

	check := NewLandWorktreeGitignoreCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when entry present, got %v: %s", result.Status, result.Message)
	}
	_ = rigPath
}

func TestLandWorktreeGitignoreCheck_Missing(t *testing.T) {
	townRoot := t.TempDir()
	createRigWithGitignore(t, townRoot, "myrig", "plugins/\n.repo.git/\n")

	check := NewLandWorktreeGitignoreCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning when entry missing, got %v: %s", result.Status, result.Message)
	}
	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d: %v", len(result.Details), result.Details)
	}
}

func TestLandWorktreeGitignoreCheck_NoGitignore(t *testing.T) {
	townRoot := t.TempDir()
	// Create rig without any .gitignore
	rigPath := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0o755); err != nil {
		t.Fatal(err)
	}

	check := NewLandWorktreeGitignoreCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning when .gitignore missing, got %v: %s", result.Status, result.Message)
	}
}

func TestLandWorktreeGitignoreCheck_MultiRig(t *testing.T) {
	townRoot := t.TempDir()
	createRigWithGitignore(t, townRoot, "goodrig", "plugins/\n.land-worktree/\n")
	createRigWithGitignore(t, townRoot, "badrig", "plugins/\n")

	check := NewLandWorktreeGitignoreCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "1 rig(s)") {
		t.Errorf("expected 1 affected rig, got message: %s", result.Message)
	}
}

func TestLandWorktreeGitignoreCheck_Fix(t *testing.T) {
	townRoot := t.TempDir()
	createRigWithGitignore(t, townRoot, "myrig", "plugins/\n")

	check := NewLandWorktreeGitignoreCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	// Run first to detect
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning before fix, got %v", result.Status)
	}

	// Fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() error: %v", err)
	}

	// Re-run should pass
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}

	// Verify file contents
	data, err := os.ReadFile(filepath.Join(townRoot, "myrig", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ".land-worktree/") {
		t.Error(".land-worktree/ not found in .gitignore after fix")
	}
	// Original content preserved
	if !strings.Contains(string(data), "plugins/") {
		t.Error("plugins/ was lost after fix")
	}
}

func TestLandWorktreeGitignoreCheck_FixCreatesFile(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0o755); err != nil {
		t.Fatal(err)
	}

	check := NewLandWorktreeGitignoreCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	check.Run(ctx)
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rigPath, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to be created: %v", err)
	}
	if !strings.Contains(string(data), ".land-worktree/") {
		t.Error(".land-worktree/ not found in created .gitignore")
	}
}

func TestLandWorktreeGitignoreCheck_NoRigs(t *testing.T) {
	townRoot := t.TempDir()

	check := NewLandWorktreeGitignoreCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rigs, got %v: %s", result.Status, result.Message)
	}
}

func TestLandWorktreeGitignoreCheck_CanFix(t *testing.T) {
	check := NewLandWorktreeGitignoreCheck()
	if !check.CanFix() {
		t.Error("expected CanFix() to return true")
	}
}

// createRigWithGitignore creates a rig directory with a .gitignore file.
func createRigWithGitignore(t *testing.T, townRoot, rigName, gitignoreContent string) string {
	t.Helper()
	rigPath := filepath.Join(townRoot, rigName)
	if err := os.MkdirAll(filepath.Join(rigPath, "crew"), 0o755); err != nil {
		t.Fatal(err)
	}
	if gitignoreContent != "" {
		if err := os.WriteFile(filepath.Join(rigPath, ".gitignore"), []byte(gitignoreContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rigPath
}
