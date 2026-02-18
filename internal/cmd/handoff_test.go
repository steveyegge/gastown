package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/workspace"
)

func setupHandoffTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestHandoffStdinFlag(t *testing.T) {
	t.Run("errors when both stdin and message provided", func(t *testing.T) {
		// Save and restore flag state
		origMessage := handoffMessage
		origStdin := handoffStdin
		defer func() {
			handoffMessage = origMessage
			handoffStdin = origStdin
		}()

		handoffMessage = "some message"
		handoffStdin = true

		err := runHandoff(handoffCmd, nil)
		if err == nil {
			t.Fatal("expected error when both --stdin and --message are set")
		}
		if !strings.Contains(err.Error(), "cannot use --stdin with --message/-m") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSessionWorkDir(t *testing.T) {
	setupHandoffTestRegistry(t)
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
			sessionName: "gt-crew-holden",
			wantDir:     townRoot + "/gastown/crew/holden",
			wantErr:     false,
		},
		{
			name:        "witness runs from witness directory",
			sessionName: "gt-witness",
			wantDir:     townRoot + "/gastown/witness",
			wantErr:     false,
		},
		{
			name:        "refinery runs from refinery/rig directory",
			sessionName: "gt-refinery",
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

// makeTestGitRepo creates a minimal git repo in a temp dir and returns its path.
// The caller is responsible for cleanup via t.Cleanup or defer os.RemoveAll.
func makeTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		// Disable background processes that hold file handles open after exit —
		// causes TempDir cleanup failures on Windows.
		{"git", "-C", dir, "config", "gc.auto", "0"},
		{"git", "-C", dir, "config", "core.fsmonitor", "false"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			t.Fatalf("git setup %v: %v", args, err)
		}
	}
	return dir
}

func TestWarnHandoffGitStatus(t *testing.T) {
	origCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origCwd) })

	t.Run("no warning on clean repo", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStdout(t, func() {
			warnHandoffGitStatus()
		})
		if output != "" {
			t.Errorf("expected no output for clean repo, got: %q", output)
		}
	})

	t.Run("warns on untracked file", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStdout(t, func() {
			warnHandoffGitStatus()
		})
		if !strings.Contains(output, "uncommitted work") {
			t.Errorf("expected warning about uncommitted work, got: %q", output)
		}
		if !strings.Contains(output, "untracked") {
			t.Errorf("expected 'untracked' in output, got: %q", output)
		}
	})

	t.Run("warns on modified tracked file", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		// Create and commit a file
		fpath := filepath.Join(dir, "tracked.txt")
		os.WriteFile(fpath, []byte("original"), 0644)
		exec.Command("git", "-C", dir, "add", ".").Run()
		exec.Command("git", "-C", dir, "commit", "-m", "add file").Run()
		// Now modify it
		os.WriteFile(fpath, []byte("modified"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStdout(t, func() {
			warnHandoffGitStatus()
		})
		if !strings.Contains(output, "uncommitted work") {
			t.Errorf("expected warning about uncommitted work, got: %q", output)
		}
		if !strings.Contains(output, "modified") {
			t.Errorf("expected 'modified' in output, got: %q", output)
		}
	})

	t.Run("no warning for .beads-only changes", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		// Only .beads/ untracked files — should be clean (excluded)
		os.MkdirAll(filepath.Join(dir, ".beads"), 0755)
		os.WriteFile(filepath.Join(dir, ".beads", "somefile.db"), []byte("db"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStdout(t, func() {
			warnHandoffGitStatus()
		})
		if output != "" {
			t.Errorf("expected no output for .beads-only changes, got: %q", output)
		}
	})

	t.Run("no warning outside git repo", func(t *testing.T) {
		os.Chdir(os.TempDir())
		output := captureStdout(t, func() {
			warnHandoffGitStatus()
		})
		if output != "" {
			t.Errorf("expected no output outside git repo, got: %q", output)
		}
	})

	t.Run("no-git-check flag suppresses warning", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		// Simulate --no-git-check by setting the flag
		origFlag := handoffNoGitCheck
		handoffNoGitCheck = true
		defer func() { handoffNoGitCheck = origFlag }()
		output := captureStdout(t, func() {
			if !handoffNoGitCheck {
				warnHandoffGitStatus()
			}
		})
		if output != "" {
			t.Errorf("expected no output with --no-git-check, got: %q", output)
		}
	})
}
