package deacon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultReapConfig(t *testing.T) {
	cfg := DefaultReapConfig()

	if cfg.IdleThreshold != DefaultIdleThreshold {
		t.Errorf("IdleThreshold = %v, want %v", cfg.IdleThreshold, DefaultIdleThreshold)
	}
	if cfg.DryRun {
		t.Error("DryRun should default to false")
	}
}

func TestReapScanResult_Empty(t *testing.T) {
	result := &ReapScanResult{
		ScannedAt: time.Now().UTC(),
		Results:   make([]*ReapResult, 0),
	}

	if result.TotalPolecats != 0 {
		t.Errorf("TotalPolecats = %d, want 0", result.TotalPolecats)
	}
	if result.Reaped != 0 {
		t.Errorf("Reaped = %d, want 0", result.Reaped)
	}
}

func TestReapResult_Fields(t *testing.T) {
	result := &ReapResult{
		Rig:            "gastown",
		Polecat:        "max",
		SessionName:    "gt-max",
		SessionKilled:  true,
		WorktreeRemoved: true,
	}

	if !result.SessionKilled {
		t.Error("SessionKilled should be true")
	}
	if !result.WorktreeRemoved {
		t.Error("WorktreeRemoved should be true")
	}
	if result.Error != "" {
		t.Errorf("Error should be empty, got %q", result.Error)
	}
}

func TestReapResult_WithPartialWork(t *testing.T) {
	result := &ReapResult{
		Rig:            "gastown",
		Polecat:        "max",
		SessionName:    "gt-max",
		SessionKilled:  true,
		WorktreeRemoved: false,
		PartialWork:    true,
		WorktreeDirty:  true,
		UnpushedCount:  2,
	}

	if result.WorktreeRemoved {
		t.Error("WorktreeRemoved should be false when partial work exists")
	}
	if !result.PartialWork {
		t.Error("PartialWork should be true")
	}
	if result.UnpushedCount != 2 {
		t.Errorf("UnpushedCount = %d, want 2", result.UnpushedCount)
	}
}

func TestListPolecatDirs_Empty(t *testing.T) {
	townRoot := t.TempDir()

	// No rig directories at all
	dirs := listPolecatDirs(townRoot)
	if len(dirs) != 0 {
		t.Errorf("expected 0 polecat dirs, got %d", len(dirs))
	}
}

func TestListPolecatDirs_WithPolecats(t *testing.T) {
	townRoot := t.TempDir()

	// Create rig with polecats directory containing two polecats
	rigDir := filepath.Join(townRoot, "testrig", "polecats")
	if err := os.MkdirAll(filepath.Join(rigDir, "max"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigDir, "toast"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a file (not a directory) — should be skipped
	if err := os.WriteFile(filepath.Join(rigDir, "not-a-polecat.txt"), []byte("skip"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a hidden directory — should be skipped
	if err := os.MkdirAll(filepath.Join(rigDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}

	dirs := listPolecatDirs(townRoot)
	if len(dirs) != 2 {
		t.Errorf("expected 2 polecat dirs, got %d: %v", len(dirs), dirs)
	}

	// Check that both polecats are found
	found := make(map[string]bool)
	for _, d := range dirs {
		found[d.Polecat] = true
		if d.Rig != "testrig" {
			t.Errorf("expected rig 'testrig', got %q", d.Rig)
		}
	}
	if !found["max"] || !found["toast"] {
		t.Errorf("expected max and toast, got %v", dirs)
	}
}

func TestListPolecatDirs_MultipleRigs(t *testing.T) {
	townRoot := t.TempDir()

	// Create two rigs with polecats
	for _, rig := range []string{"rig-a", "rig-b"} {
		polecatsDir := filepath.Join(townRoot, rig, "polecats")
		if err := os.MkdirAll(filepath.Join(polecatsDir, "worker1"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	dirs := listPolecatDirs(townRoot)
	if len(dirs) != 2 {
		t.Errorf("expected 2 polecat dirs across 2 rigs, got %d", len(dirs))
	}
}

func TestListPolecatDirs_SkipsNonRigDirs(t *testing.T) {
	townRoot := t.TempDir()

	// Create directories that look like rigs but aren't (no polecats subdir)
	if err := os.MkdirAll(filepath.Join(townRoot, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "deacon"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a hidden dir at town level
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}

	dirs := listPolecatDirs(townRoot)
	if len(dirs) != 0 {
		t.Errorf("expected 0 polecat dirs (no rigs), got %d", len(dirs))
	}
}

func TestPolecatWorktreePath_NewStructure(t *testing.T) {
	townRoot := t.TempDir()

	// Create new structure: rig/polecats/<name>/<rigname>/
	worktreePath := filepath.Join(townRoot, "testrig", "polecats", "max", "testrig")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: /fake"), 0644); err != nil {
		t.Fatal(err)
	}

	got := polecatWorktreePath(townRoot, "testrig", "max")
	if got != worktreePath {
		t.Errorf("polecatWorktreePath() = %q, want %q", got, worktreePath)
	}
}

func TestPolecatWorktreePath_OldStructure(t *testing.T) {
	townRoot := t.TempDir()

	// Create old structure: rig/polecats/<name>/
	worktreePath := filepath.Join(townRoot, "testrig", "polecats", "max")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: /fake"), 0644); err != nil {
		t.Fatal(err)
	}

	got := polecatWorktreePath(townRoot, "testrig", "max")
	if got != worktreePath {
		t.Errorf("polecatWorktreePath() = %q, want %q", got, worktreePath)
	}
}

func TestPolecatWorktreePath_NoWorktree(t *testing.T) {
	townRoot := t.TempDir()

	// Directory exists but no .git
	dirPath := filepath.Join(townRoot, "testrig", "polecats", "max")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatal(err)
	}

	got := polecatWorktreePath(townRoot, "testrig", "max")
	if got != "" {
		t.Errorf("polecatWorktreePath() = %q, want empty (no .git)", got)
	}
}

func TestPolecatWorktreePath_Nonexistent(t *testing.T) {
	townRoot := t.TempDir()

	got := polecatWorktreePath(townRoot, "testrig", "ghost")
	if got != "" {
		t.Errorf("polecatWorktreePath() = %q, want empty", got)
	}
}

func TestIsBeadClosed_ClosedStatus(t *testing.T) {
	// Test the status check logic directly
	for _, status := range []string{"closed", "done", "merged"} {
		if !isClosedStatus(status) {
			t.Errorf("isClosedStatus(%q) = false, want true", status)
		}
	}
}

func TestIsBeadClosed_OpenStatus(t *testing.T) {
	for _, status := range []string{"open", "hooked", "in_progress", "working"} {
		if isClosedStatus(status) {
			t.Errorf("isClosedStatus(%q) = true, want false", status)
		}
	}
}
