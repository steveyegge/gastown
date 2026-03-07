package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestListPolecatNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	polecatsDir := filepath.Join(dir, "polecats")
	if err := os.MkdirAll(polecatsDir, 0o755); err != nil {
		t.Fatalf("creating polecats dir: %v", err)
	}

	// Create some polecat directories
	for _, name := range []string{"amber", "onyx", "pearl"} {
		if err := os.Mkdir(filepath.Join(polecatsDir, name), 0o755); err != nil {
			t.Fatalf("creating polecat dir %s: %v", name, err)
		}
	}

	// Create a hidden directory (should be excluded)
	if err := os.Mkdir(filepath.Join(polecatsDir, ".beads"), 0o755); err != nil {
		t.Fatalf("creating .beads dir: %v", err)
	}

	// Create a regular file (should be excluded — not a dir)
	if err := os.WriteFile(filepath.Join(polecatsDir, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("creating file: %v", err)
	}

	r := &rig.Rig{Path: dir}
	names := listPolecatNames(r)

	if len(names) != 3 {
		t.Fatalf("listPolecatNames() returned %d names, want 3: %v", len(names), names)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, want := range []string{"amber", "onyx", "pearl"} {
		if !nameSet[want] {
			t.Errorf("expected %q in names, got %v", want, names)
		}
	}
}

func TestListPolecatNames_NoPolecatsDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := &rig.Rig{Path: dir}
	names := listPolecatNames(r)
	if len(names) != 0 {
		t.Errorf("listPolecatNames() returned %v, want empty", names)
	}
}

func TestIsHiddenDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{".beads", true},
		{".git", true},
		{"amber", false},
		{"onyx", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHiddenDir(tt.name); got != tt.want {
				t.Errorf("isHiddenDir(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestFormatWorkspaceState(t *testing.T) {
	t.Parallel()

	// Verify it doesn't panic for various states
	states := []string{"running", "Running", "stopped", "Stopped", "creating", ""}
	for _, s := range states {
		_ = formatWorkspaceState(s) // should not panic
	}
}

func TestPolecatDiscoverCmdRegistered(t *testing.T) {
	t.Parallel()
	// Verify the discover subcommand is registered under polecatCmd
	found := false
	for _, cmd := range polecatCmd.Commands() {
		if cmd.Use == "discover <rig>" {
			found = true
			// Verify flags exist
			if cmd.Flag("reconcile") == nil {
				t.Error("discover command missing --reconcile flag")
			}
			if cmd.Flag("dry-run") == nil {
				t.Error("discover command missing --dry-run flag")
			}
			if cmd.Flag("json") == nil {
				t.Error("discover command missing --json flag")
			}
			break
		}
	}
	if !found {
		t.Error("polecatDiscoverCmd not registered as subcommand of polecatCmd")
	}
}
