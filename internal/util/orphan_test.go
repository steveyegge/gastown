//go:build !windows

package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEtime(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		// MM:SS format
		{"00:30", 30, false},
		{"01:00", 60, false},
		{"01:23", 83, false},
		{"59:59", 3599, false},

		// HH:MM:SS format
		{"00:01:00", 60, false},
		{"01:00:00", 3600, false},
		{"01:02:03", 3723, false},
		{"23:59:59", 86399, false},

		// DD-HH:MM:SS format
		{"1-00:00:00", 86400, false},
		{"2-01:02:03", 176523, false},
		{"7-12:30:45", 649845, false},

		// Edge cases
		{"00:00", 0, false},
		{"0-00:00:00", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseEtime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEtime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseEtime(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFindOrphanedClaudeProcesses(t *testing.T) {
	// Live test that checks for orphaned processes on the current system.
	// Should not fail — just returns whatever orphans exist (likely none in CI).
	// Also verifies that processes with a TTY are excluded (only TTY=? reported).
	orphans, err := FindOrphanedClaudeProcesses()
	if err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses() error = %v", err)
	}

	t.Logf("Found %d orphaned claude processes", len(orphans))
	for _, o := range orphans {
		t.Logf("  PID %d: %s (verify TTY=? in 'ps aux')", o.PID, o.Cmd)
	}
}

func TestGetProcessCwd(t *testing.T) {
	// Our own process should have a valid cwd
	cwd := getProcessCwd(os.Getpid())
	if cwd == "" {
		t.Fatal("getProcessCwd(self) returned empty string")
	}
	// Verify it matches os.Getwd
	expected, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error: %v", err)
	}
	if cwd != expected {
		t.Errorf("getProcessCwd(self) = %q, want %q", cwd, expected)
	}
}

func TestIsInGasTownWorkspace(t *testing.T) {
	// NOTE: This test uses os.Chdir on the process-global cwd.
	// Do NOT add t.Parallel() here or to any test in this file—concurrent
	// tests sharing the same process would race on the working directory.

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Create a temporary directory structure simulating a Gas Town workspace
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	townJSON := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townJSON, []byte(`{"name":"test-town"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Move to a non-workspace temp dir first, so the "not in workspace" check
	// works even when tests are run from inside a real GT workspace.
	nonWorkspaceDir := t.TempDir()
	if err := os.Chdir(nonWorkspaceDir); err != nil {
		t.Fatal(err)
	}

	// Our process is NOT in the temp workspace, so should return false
	if isInGasTownWorkspace(os.Getpid()) {
		t.Error("isInGasTownWorkspace(self) = true, want false (not in a GT workspace)")
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	if !isInGasTownWorkspace(os.Getpid()) {
		t.Error("isInGasTownWorkspace(self) = false, want true (in GT workspace root)")
	}

	// Test from a subdirectory of the workspace
	subDir := filepath.Join(tmpDir, "polecats", "test-polecat")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}
	if !isInGasTownWorkspace(os.Getpid()) {
		t.Error("isInGasTownWorkspace(self) = false, want true (in GT workspace subdir)")
	}
}
