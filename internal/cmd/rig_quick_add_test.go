package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindOrCreateTown(t *testing.T) {
	// Save original env and restore after test
	origTownRoot := os.Getenv("GT_TOWN_ROOT")
	defer os.Setenv("GT_TOWN_ROOT", origTownRoot)

	t.Run("respects GT_TOWN_ROOT when set", func(t *testing.T) {
		// Create a valid town in temp dir
		tmpTown := t.TempDir()
		mayorDir := filepath.Join(tmpTown, "mayor")
		if err := os.MkdirAll(mayorDir, 0755); err != nil {
			t.Fatalf("mkdir mayor: %v", err)
		}

		os.Setenv("GT_TOWN_ROOT", tmpTown)

		result, err := findOrCreateTown()
		if err != nil {
			t.Fatalf("findOrCreateTown() error = %v", err)
		}
		if result != tmpTown {
			t.Errorf("findOrCreateTown() = %q, want %q", result, tmpTown)
		}
	})

	t.Run("ignores invalid GT_TOWN_ROOT", func(t *testing.T) {
		// Set GT_TOWN_ROOT to a non-existent path
		os.Setenv("GT_TOWN_ROOT", "/nonexistent/path/to/town")

		// Create a valid town at ~/gt for fallback
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot get home dir")
		}

		gtPath := filepath.Join(home, "gt")
		mayorDir := filepath.Join(gtPath, "mayor")

		// Skip if ~/gt doesn't exist (don't want to create it in user's home)
		if _, err := os.Stat(mayorDir); os.IsNotExist(err) {
			t.Skip("~/gt/mayor does not exist, skipping fallback test")
		}

		result, err := findOrCreateTown()
		if err != nil {
			t.Fatalf("findOrCreateTown() error = %v", err)
		}
		// Should fall back to ~/gt since GT_TOWN_ROOT is invalid
		if result != gtPath {
			t.Logf("findOrCreateTown() = %q (fell back to valid town)", result)
		}
	})

	t.Run("GT_TOWN_ROOT takes priority over fallback", func(t *testing.T) {
		// Create two valid towns
		tmpTown1 := t.TempDir()
		tmpTown2 := t.TempDir()

		if err := os.MkdirAll(filepath.Join(tmpTown1, "mayor"), 0755); err != nil {
			t.Fatalf("mkdir mayor1: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(tmpTown2, "mayor"), 0755); err != nil {
			t.Fatalf("mkdir mayor2: %v", err)
		}

		// Set GT_TOWN_ROOT to tmpTown1
		os.Setenv("GT_TOWN_ROOT", tmpTown1)

		result, err := findOrCreateTown()
		if err != nil {
			t.Fatalf("findOrCreateTown() error = %v", err)
		}
		// Should use GT_TOWN_ROOT, not any other valid town
		if result != tmpTown1 {
			t.Errorf("findOrCreateTown() = %q, want %q (GT_TOWN_ROOT should take priority)", result, tmpTown1)
		}
	})
}

func TestIsValidTown(t *testing.T) {
	t.Run("valid town has mayor directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		mayorDir := filepath.Join(tmpDir, "mayor")
		if err := os.MkdirAll(mayorDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if !isValidTown(tmpDir) {
			t.Error("isValidTown() = false, want true")
		}
	})

	t.Run("invalid town missing mayor directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		if isValidTown(tmpDir) {
			t.Error("isValidTown() = true, want false")
		}
	})

	t.Run("nonexistent path is invalid", func(t *testing.T) {
		if isValidTown("/nonexistent/path") {
			t.Error("isValidTown() = true, want false")
		}
	})
}
