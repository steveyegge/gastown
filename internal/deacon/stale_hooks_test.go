package deacon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssigneeToSessionName(t *testing.T) {
	tests := []struct {
		assignee string
		want     string
	}{
		{"deacon", "hq-deacon"},
		{"mayor", "hq-mayor"},
		{"gastown/witness", "gt-witness"},
		{"gastown/refinery", "gt-refinery"},
		{"gastown/polecats/max", "gt-max"},
		{"gastown/crew/joe", "gt-crew-joe"},
		{"", ""},
		{"unknown", ""},
		{"gastown/unknown/agent", ""},
		{"a/b/c/d", ""},
	}

	for _, tt := range tests {
		t.Run(tt.assignee, func(t *testing.T) {
			got := assigneeToSessionName(tt.assignee)
			if got != tt.want {
				t.Errorf("assigneeToSessionName(%q) = %q, want %q", tt.assignee, got, tt.want)
			}
		})
	}
}

func TestAssigneeToWorktreePath_InvalidFormats(t *testing.T) {
	townRoot := t.TempDir()

	tests := []struct {
		name     string
		assignee string
	}{
		{"empty", ""},
		{"single part", "deacon"},
		{"two parts", "gastown/witness"},
		{"four parts", "a/b/c/d"},
		{"unknown agent type", "gastown/unknown/agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assigneeToWorktreePath(townRoot, tt.assignee)
			if got != "" {
				t.Errorf("assigneeToWorktreePath(%q, %q) = %q, want empty", townRoot, tt.assignee, got)
			}
		})
	}
}

func TestAssigneeToWorktreePath_NewStructure(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "testrig"

	// Create new-structure worktree: townRoot/testrig/polecats/max/testrig/
	worktreePath := filepath.Join(townRoot, rigName, "polecats", "max", rigName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}
	// Create .git file (worktree indicator)
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: /fake"), 0644); err != nil {
		t.Fatal(err)
	}

	got := assigneeToWorktreePath(townRoot, "testrig/polecats/max")
	if got != worktreePath {
		t.Errorf("assigneeToWorktreePath() = %q, want %q", got, worktreePath)
	}
}

func TestAssigneeToWorktreePath_OldStructure(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "testrig"

	// Create old-structure worktree: townRoot/testrig/polecats/max/
	worktreePath := filepath.Join(townRoot, rigName, "polecats", "max")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}
	// Create .git file (worktree indicator)
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: /fake"), 0644); err != nil {
		t.Fatal(err)
	}

	got := assigneeToWorktreePath(townRoot, "testrig/polecats/max")
	if got != worktreePath {
		t.Errorf("assigneeToWorktreePath() = %q, want %q", got, worktreePath)
	}
}

func TestAssigneeToWorktreePath_CrewWorker(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "testrig"

	// Create crew worktree: townRoot/testrig/crew/joe/testrig/
	worktreePath := filepath.Join(townRoot, rigName, "crew", "joe", rigName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: /fake"), 0644); err != nil {
		t.Fatal(err)
	}

	got := assigneeToWorktreePath(townRoot, "testrig/crew/joe")
	if got != worktreePath {
		t.Errorf("assigneeToWorktreePath() = %q, want %q", got, worktreePath)
	}
}

func TestAssigneeToWorktreePath_NoWorktree(t *testing.T) {
	townRoot := t.TempDir()

	// Directory exists but no .git -> not a worktree
	dirPath := filepath.Join(townRoot, "testrig", "polecats", "max")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatal(err)
	}

	got := assigneeToWorktreePath(townRoot, "testrig/polecats/max")
	if got != "" {
		t.Errorf("assigneeToWorktreePath() = %q, want empty (no .git)", got)
	}
}

func TestCheckWorktreeState_CleanRepo(t *testing.T) {
	// Create a real git repo to test against
	tmpDir := t.TempDir()
	townRoot := tmpDir
	rigName := "testrig"

	worktreePath := filepath.Join(townRoot, rigName, "polecats", "max", rigName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	// Initialize a real git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = worktreePath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	result := &StaleHookResult{}
	checkWorktreeState(townRoot, "testrig/polecats/max", result)

	if result.PartialWork {
		t.Error("expected no partial work for clean repo")
	}
	if result.WorktreeDirty {
		t.Error("expected worktree not dirty for clean repo")
	}
	if result.UnpushedCount != 0 {
		t.Errorf("expected 0 unpushed commits, got %d", result.UnpushedCount)
	}
}

func TestCheckWorktreeState_DirtyRepo(t *testing.T) {
	tmpDir := t.TempDir()
	townRoot := tmpDir
	rigName := "testrig"

	worktreePath := filepath.Join(townRoot, rigName, "polecats", "max", rigName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	// Initialize git repo with a commit, then add uncommitted file
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = worktreePath
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	// Create an uncommitted file
	if err := os.WriteFile(filepath.Join(worktreePath, "dirty.txt"), []byte("uncommitted"), 0644); err != nil {
		t.Fatal(err)
	}

	result := &StaleHookResult{}
	checkWorktreeState(townRoot, "testrig/polecats/max", result)

	if !result.PartialWork {
		t.Error("expected partial work for dirty repo")
	}
	if !result.WorktreeDirty {
		t.Error("expected worktree dirty")
	}
}

func TestCheckWorktreeState_InvalidAssignee(t *testing.T) {
	townRoot := t.TempDir()

	result := &StaleHookResult{}
	checkWorktreeState(townRoot, "invalid", result)

	// Should not populate any fields for unresolvable assignee
	if result.PartialWork {
		t.Error("expected no partial work for invalid assignee")
	}
	if result.WorktreeError != "" {
		t.Errorf("expected no worktree error, got %q", result.WorktreeError)
	}
}

func TestCheckWorktreeState_NonexistentPath(t *testing.T) {
	townRoot := t.TempDir()

	result := &StaleHookResult{}
	checkWorktreeState(townRoot, "testrig/polecats/ghost", result)

	// Assignee format is valid but path doesn't exist
	if result.PartialWork {
		t.Error("expected no partial work for nonexistent path")
	}
}

func TestDefaultStaleHookConfig(t *testing.T) {
	cfg := DefaultStaleHookConfig()

	if cfg.MaxAge != 1*60*60*1e9 { // 1 hour in nanoseconds
		t.Errorf("MaxAge = %v, want 1h", cfg.MaxAge)
	}
	if cfg.DryRun {
		t.Error("DryRun should default to false")
	}
}

func TestStaleHookResult_PartialWorkFields(t *testing.T) {
	result := &StaleHookResult{
		BeadID:        "gt-abc",
		Title:         "test bead",
		Assignee:      "gastown/polecats/max",
		PartialWork:   true,
		WorktreeDirty: true,
		UnpushedCount: 3,
	}

	if !result.PartialWork {
		t.Error("PartialWork should be true")
	}
	if !result.WorktreeDirty {
		t.Error("WorktreeDirty should be true")
	}
	if result.UnpushedCount != 3 {
		t.Errorf("UnpushedCount = %d, want 3", result.UnpushedCount)
	}
}
