package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWorktreeGitdirCheck(t *testing.T) {
	check := NewWorktreeGitdirCheck()

	if check.Name() != "worktree-gitdir-valid" {
		t.Errorf("expected name 'worktree-gitdir-valid', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestWorktreeGitdirCheck_NoRigs(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rigs exist, got %v", result.Status)
	}
}

func TestWorktreeGitdirCheck_ValidWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig structure with config.json
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a fake .repo.git/worktrees/rig directory
	worktreeEntry := filepath.Join(rigDir, ".repo.git", "worktrees", "rig")
	if err := os.MkdirAll(worktreeEntry, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a .git file in refinery/rig that points to the worktree entry
	gitFile := filepath.Join(rigDir, "refinery", "rig", ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+worktreeEntry+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid worktree, got %v: %s", result.Status, result.Message)
	}
}

func TestWorktreeGitdirCheck_BrokenGitdir_MissingBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig structure
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .git file pointing to non-existent .repo.git
	gitFile := filepath.Join(rigDir, "refinery", "rig", ".git")
	brokenPath := filepath.Join(rigDir, ".repo.git", "worktrees", "rig")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+brokenPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for broken gitdir, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "broken gitdir") {
		t.Errorf("expected message about broken gitdir, got %q", result.Message)
	}
	if len(result.Details) == 0 {
		t.Error("expected details about the broken worktree")
	}
	if !strings.Contains(result.Details[0], ".repo.git missing") {
		t.Errorf("expected detail about missing .repo.git, got %q", result.Details[0])
	}
}

func TestWorktreeGitdirCheck_BrokenGitdir_MissingWorktreeEntry(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig structure
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .repo.git but WITHOUT the worktree entry
	if err := os.MkdirAll(filepath.Join(rigDir, ".repo.git", "worktrees"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create .git file pointing to missing worktree entry
	gitFile := filepath.Join(rigDir, "refinery", "rig", ".git")
	brokenPath := filepath.Join(rigDir, ".repo.git", "worktrees", "rig")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+brokenPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing worktree entry, got %v", result.Status)
	}
	if len(result.Details) == 0 {
		t.Error("expected details about the broken worktree")
	}
	if !strings.Contains(result.Details[0], "worktree entry missing") {
		t.Errorf("expected detail about missing worktree entry, got %q", result.Details[0])
	}
}

func TestWorktreeGitdirCheck_CloneNotWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig with refinery/rig as a regular clone (directory .git, not file)
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should pass - regular clones (directory .git) are not checked
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for regular clone, got %v: %s", result.Status, result.Message)
	}
}

func TestWorktreeGitdirCheck_MalformedGitFile(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a malformed .git file
	gitFile := filepath.Join(rigDir, "refinery", "rig", ".git")
	if err := os.WriteFile(gitFile, []byte("not a valid gitdir reference\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for malformed .git file, got %v", result.Status)
	}
	if len(result.Details) == 0 {
		t.Error("expected details about malformed .git file")
	}
	if !strings.Contains(result.Details[0], "malformed") {
		t.Errorf("expected detail about malformed file, got %q", result.Details[0])
	}
}

func TestWorktreeGitdirCheck_PolecatWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig structure with a polecat worktree (new structure)
	rigDir := filepath.Join(tmpDir, rigName)
	polecatDir := filepath.Join(rigDir, "polecats", "alpha", rigName)
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create broken .git file for polecat
	gitFile := filepath.Join(polecatDir, ".git")
	brokenPath := filepath.Join(rigDir, ".repo.git", "worktrees", "alpha")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+brokenPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for broken polecat worktree, got %v", result.Status)
	}
}

func TestWorktreeGitdirCheck_RigFilter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two rigs, one with broken worktree
	for _, rigName := range []string{"goodrig", "badrig"} {
		rigDir := filepath.Join(tmpDir, rigName)
		if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create broken .git file only in badrig
	gitFile := filepath.Join(tmpDir, "badrig", "refinery", "rig", ".git")
	brokenPath := filepath.Join(tmpDir, "badrig", ".repo.git", "worktrees", "rig")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+brokenPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()

	// When checking only goodrig, should pass
	ctx := &CheckContext{TownRoot: tmpDir, RigName: "goodrig"}
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when filtering to goodrig, got %v", result.Status)
	}

	// When checking badrig, should fail
	check2 := NewWorktreeGitdirCheck()
	ctx2 := &CheckContext{TownRoot: tmpDir, RigName: "badrig"}
	result2 := check2.Run(ctx2)
	if result2.Status != StatusError {
		t.Errorf("expected StatusError when filtering to badrig, got %v", result2.Status)
	}
}

// ── New tests for hq-c6u: relocation and deacon dogs ──────────────────── //

func TestWorktreeGitdirCheck_RelocatedWorktree(t *testing.T) {
	// Simulate rsync from /old/prefix/gt to tmpDir (new town root).
	// The .git file contains an absolute path with the old prefix,
	// but .repo.git exists at the new location.
	tmpDir := t.TempDir()
	rigName := "myrig"

	// Create rig with .repo.git at the new (correct) location
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigDir, ".repo.git", "worktrees"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create .git file with OLD prefix path (simulating rsync from another machine)
	gitFile := filepath.Join(rigDir, "refinery", "rig", ".git")
	oldPath := "/Users/olduser/gt/" + rigName + "/.repo.git/worktrees/rig"
	if err := os.WriteFile(gitFile, []byte("gitdir: "+oldPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for relocated worktree, got %v", result.Status)
	}
	if len(result.Details) == 0 {
		t.Fatal("expected details about relocated worktree")
	}
	// Should say "relocated" not ".repo.git missing"
	if !strings.Contains(result.Details[0], "relocated") {
		t.Errorf("expected 'relocated' in detail, got %q", result.Details[0])
	}

	// Verify the corrected bare repo was found
	if len(check.brokenWorktrees) != 1 {
		t.Fatalf("expected 1 broken worktree, got %d", len(check.brokenWorktrees))
	}
	bw := check.brokenWorktrees[0]
	expectedCorrected := filepath.Join(rigDir, ".repo.git")
	if bw.correctedBareRepo != expectedCorrected {
		t.Errorf("expected correctedBareRepo=%q, got %q", expectedCorrected, bw.correctedBareRepo)
	}
}

func TestWorktreeGitdirCheck_DeaconDogs(t *testing.T) {
	// Simulate deacon/dogs/<dogname>/<rigname>/.git pointing to stale paths.
	tmpDir := t.TempDir()
	rigName := "myrig"

	// Create the rig with .repo.git
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, ".repo.git", "worktrees"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create deacon/dogs/alpha/myrig/ with a broken .git file
	dogWtDir := filepath.Join(tmpDir, "deacon", "dogs", "alpha", rigName)
	if err := os.MkdirAll(dogWtDir, 0755); err != nil {
		t.Fatal(err)
	}

	gitFile := filepath.Join(dogWtDir, ".git")
	oldPath := "/old/prefix/gt/" + rigName + "/.repo.git/worktrees/myrig1"
	if err := os.WriteFile(gitFile, []byte("gitdir: "+oldPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for broken deacon dog worktree, got %v", result.Status)
	}
	if len(result.Details) == 0 {
		t.Fatal("expected details about broken deacon dog worktree")
	}
	// Should mention deacon/dogs path
	if !strings.Contains(result.Details[0], "deacon/dogs/alpha") {
		t.Errorf("expected deacon/dogs/alpha in detail, got %q", result.Details[0])
	}
	// Should identify as relocated (since .repo.git exists at correct location)
	if !strings.Contains(result.Details[0], "relocated") {
		t.Errorf("expected 'relocated' in detail, got %q", result.Details[0])
	}
}

func TestWorktreeGitdirCheck_DeaconDogs_MultipleDogs(t *testing.T) {
	// Multiple dogs with broken worktrees for the same rig.
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(filepath.Join(rigDir, ".repo.git", "worktrees"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"repo":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create 3 dogs with broken worktrees
	for _, dog := range []string{"alpha", "bravo", "charlie"} {
		dogWtDir := filepath.Join(tmpDir, "deacon", "dogs", dog, rigName)
		if err := os.MkdirAll(dogWtDir, 0755); err != nil {
			t.Fatal(err)
		}
		gitFile := filepath.Join(dogWtDir, ".git")
		oldPath := "/old/path/" + rigName + "/.repo.git/worktrees/" + rigName + "_" + dog
		if err := os.WriteFile(gitFile, []byte("gitdir: "+oldPath+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "3 worktree") {
		t.Errorf("expected 3 broken worktrees, got %q", result.Message)
	}
}

func TestWorktreeGitdirCheck_NoDeaconDogs(t *testing.T) {
	// Town with no deacon/dogs should still pass.
	tmpDir := t.TempDir()

	check := NewWorktreeGitdirCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for town with no deacon/dogs, got %v", result.Status)
	}
}
