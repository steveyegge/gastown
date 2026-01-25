package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/workspace"
)

func TestSessionWorkDir(t *testing.T) {
	townRoot := "/home/test/gt"

	tests := []struct {
		name        string
		sessionName string
		wantDir     string
		wantErr     bool
	}{
		{
			name:        "mayor runs from mayor subdirectory",
			sessionName: "hq-mayor",
			wantDir:     townRoot + "/mayor",
			wantErr:     false,
		},
		{
			name:        "deacon runs from deacon subdirectory",
			sessionName: "hq-deacon",
			wantDir:     townRoot + "/deacon",
			wantErr:     false,
		},
		{
			name:        "crew runs from crew subdirectory",
			sessionName: "gt-gastown-crew-holden",
			wantDir:     townRoot + "/gastown/crew/holden",
			wantErr:     false,
		},
		{
			name:        "witness runs from witness directory",
			sessionName: "gt-gastown-witness",
			wantDir:     townRoot + "/gastown/witness",
			wantErr:     false,
		},
		{
			name:        "refinery runs from refinery/rig directory",
			sessionName: "gt-gastown-refinery",
			wantDir:     townRoot + "/gastown/refinery/rig",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, err := sessionWorkDir(tt.sessionName, townRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("sessionWorkDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotDir != tt.wantDir {
				t.Errorf("sessionWorkDir() = %q, want %q", gotDir, tt.wantDir)
			}
		})
	}
}

func TestDetectTownRootFromCwd_EnvFallback(t *testing.T) {
	// Save original env vars and restore after test
	origTownRoot := os.Getenv("GT_TOWN_ROOT")
	origRoot := os.Getenv("GT_ROOT")
	defer func() {
		os.Setenv("GT_TOWN_ROOT", origTownRoot)
		os.Setenv("GT_ROOT", origRoot)
	}()

	// Create a temp directory that looks like a valid town
	tmpTown := t.TempDir()
	mayorDir := filepath.Join(tmpTown, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("creating mayor dir: %v", err)
	}
	townJSON := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townJSON, []byte(`{"name": "test-town"}`), 0644); err != nil {
		t.Fatalf("creating town.json: %v", err)
	}

	// Clear both env vars initially
	os.Setenv("GT_TOWN_ROOT", "")
	os.Setenv("GT_ROOT", "")

	t.Run("uses GT_TOWN_ROOT when cwd detection fails", func(t *testing.T) {
		// Set GT_TOWN_ROOT to our temp town
		os.Setenv("GT_TOWN_ROOT", tmpTown)
		os.Setenv("GT_ROOT", "")

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should use GT_TOWN_ROOT fallback)", result, tmpTown)
		}
	})

	t.Run("uses GT_ROOT when GT_TOWN_ROOT not set", func(t *testing.T) {
		// Set only GT_ROOT
		os.Setenv("GT_TOWN_ROOT", "")
		os.Setenv("GT_ROOT", tmpTown)

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should use GT_ROOT fallback)", result, tmpTown)
		}
	})

	t.Run("prefers GT_TOWN_ROOT over GT_ROOT", func(t *testing.T) {
		// Create another temp town for GT_ROOT
		anotherTown := t.TempDir()
		anotherMayor := filepath.Join(anotherTown, "mayor")
		os.MkdirAll(anotherMayor, 0755)
		os.WriteFile(filepath.Join(anotherMayor, "town.json"), []byte(`{"name": "other-town"}`), 0644)

		// Set both env vars
		os.Setenv("GT_TOWN_ROOT", tmpTown)
		os.Setenv("GT_ROOT", anotherTown)

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should prefer GT_TOWN_ROOT)", result, tmpTown)
		}
	})

	t.Run("ignores invalid GT_TOWN_ROOT", func(t *testing.T) {
		// Set GT_TOWN_ROOT to non-existent path, GT_ROOT to valid
		os.Setenv("GT_TOWN_ROOT", "/nonexistent/path/to/town")
		os.Setenv("GT_ROOT", tmpTown)

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should skip invalid GT_TOWN_ROOT and use GT_ROOT)", result, tmpTown)
		}
	})

	t.Run("uses secondary marker when primary missing", func(t *testing.T) {
		// Create a temp town with only mayor/ directory (no town.json)
		secondaryTown := t.TempDir()
		mayorOnlyDir := filepath.Join(secondaryTown, workspace.SecondaryMarker)
		os.MkdirAll(mayorOnlyDir, 0755)

		os.Setenv("GT_TOWN_ROOT", secondaryTown)
		os.Setenv("GT_ROOT", "")

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != secondaryTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should accept secondary marker)", result, secondaryTown)
		}
	})
}
