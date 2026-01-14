package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeStateCheck_ValidJSON(t *testing.T) {
	t.Parallel()

	// Create temp workspace
	townRoot := t.TempDir()
	runtimeDir := filepath.Join(townRoot, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create valid JSON file
	validJSON := `{"state": "running", "pid": 12345}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "daemon.json"), []byte(validJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRuntimeStateCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid JSON, got %v: %s", result.Status, result.Message)
	}
}

func TestRuntimeStateCheck_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Create temp workspace
	townRoot := t.TempDir()
	runtimeDir := filepath.Join(townRoot, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create invalid JSON file
	invalidJSON := `{invalid json`
	if err := os.WriteFile(filepath.Join(runtimeDir, "broken.json"), []byte(invalidJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRuntimeStateCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status == StatusOK {
		t.Error("expected non-OK status for invalid JSON")
	}
	if len(check.invalidFiles) != 1 {
		t.Errorf("expected 1 invalid file, got %d", len(check.invalidFiles))
	}
}

func TestRuntimeStateCheck_EmptyFile(t *testing.T) {
	t.Parallel()

	// Create temp workspace
	townRoot := t.TempDir()
	runtimeDir := filepath.Join(townRoot, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty JSON file
	if err := os.WriteFile(filepath.Join(runtimeDir, "empty.json"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRuntimeStateCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	if result.Status == StatusOK {
		t.Error("expected non-OK status for empty JSON file")
	}
}

func TestRuntimeStateCheck_Fix(t *testing.T) {
	t.Parallel()

	// Create temp workspace
	townRoot := t.TempDir()
	runtimeDir := filepath.Join(townRoot, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create invalid JSON file
	brokenFile := filepath.Join(runtimeDir, "broken.json")
	if err := os.WriteFile(brokenFile, []byte(`{invalid`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRuntimeStateCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	// Run first to detect the issue
	_ = check.Run(ctx)

	// Fix should remove the file
	if err := check.Fix(ctx); err != nil {
		t.Errorf("Fix failed: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(brokenFile); !os.IsNotExist(err) {
		t.Error("Fix did not remove invalid file")
	}
}

func TestRuntimeStateCheck_NoRuntimeDir(t *testing.T) {
	t.Parallel()

	// Create temp workspace without .runtime
	townRoot := t.TempDir()

	check := NewRuntimeStateCheck()
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	// Should pass when no .runtime directory exists
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no .runtime dir, got %v", result.Status)
	}
}
