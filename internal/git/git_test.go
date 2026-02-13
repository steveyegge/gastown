package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Configure user for commits
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	_ = cmd.Run()

	// Create initial commit
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	_ = cmd.Run()

	return dir
}

func TestIsRepo(t *testing.T) {
	dir := t.TempDir()
	g := NewGit(dir)

	if g.IsRepo() {
		t.Fatal("expected IsRepo to be false for empty dir")
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	if !g.IsRepo() {
		t.Fatal("expected IsRepo to be true after git init")
	}
}

func TestCloneWithReferenceCreatesAlternates(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	if err := exec.Command("git", "init", src).Run(); err != nil {
		t.Fatalf("init src: %v", err)
	}
	_ = exec.Command("git", "-C", src, "config", "user.email", "test@test.com").Run()
	_ = exec.Command("git", "-C", src, "config", "user.name", "Test User").Run()

	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_ = exec.Command("git", "-C", src, "add", ".").Run()
	_ = exec.Command("git", "-C", src, "commit", "-m", "initial").Run()

	g := NewGit(tmp)
	if err := g.CloneWithReference(src, dst, src); err != nil {
		t.Fatalf("CloneWithReference: %v", err)
	}

	alternates := filepath.Join(dst, ".git", "objects", "info", "alternates")
	if _, err := os.Stat(alternates); err != nil {
		t.Fatalf("expected alternates file: %v", err)
	}
}

func TestCloneWithReferencePreservesSymlinks(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	// Create test repo with symlink
	if err := exec.Command("git", "init", src).Run(); err != nil {
		t.Fatalf("init src: %v", err)
	}
	_ = exec.Command("git", "-C", src, "config", "user.email", "test@test.com").Run()
	_ = exec.Command("git", "-C", src, "config", "user.name", "Test User").Run()

	// Create a directory and a symlink to it
	targetDir := filepath.Join(src, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "file.txt"), []byte("content\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	linkPath := filepath.Join(src, "link")
	if err := os.Symlink("target", linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_ = exec.Command("git", "-C", src, "add", ".").Run()
	_ = exec.Command("git", "-C", src, "commit", "-m", "initial").Run()

	// Clone with reference
	g := NewGit(tmp)
	if err := g.CloneWithReference(src, dst, src); err != nil {
		t.Fatalf("CloneWithReference: %v", err)
	}

	// Verify symlink was preserved
	dstLink := filepath.Join(dst, "link")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected %s to be a symlink, got mode %v", dstLink, info.Mode())
	}

	// Verify symlink target is correct
	target, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "target" {
		t.Errorf("expected symlink target 'target', got %q", target)
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)

	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	// Modern git uses "main", older uses "master"
	if branch != "main" && branch != "master" {
		t.Errorf("branch = %q, want main or master", branch)
	}
}

func TestStatus(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)

	// Should be clean initially
	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Clean {
		t.Error("expected clean status")
	}

	// Add an untracked file
	testFile := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(testFile, []byte("new"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	status, err = g.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Clean {
		t.Error("expected dirty status")
	}
	if len(status.Untracked) != 1 {
		t.Errorf("untracked = %d, want 1", len(status.Untracked))
	}
}

func TestAddAndCommit(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)

	// Create a new file
	testFile := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(testFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Add and commit
	if err := g.Add("new.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("add new file"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Should be clean
	status, err := g.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Clean {
		t.Error("expected clean after commit")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)

	has, err := g.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if has {
		t.Error("expected no changes initially")
	}

	// Modify a file
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	has, err = g.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges: %v", err)
	}
	if !has {
		t.Error("expected changes after modify")
	}
}

func TestCheckout(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)

	// Create a new branch
	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Checkout the new branch
	if err := g.Checkout("feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	branch, _ := g.CurrentBranch()
	if branch != "feature" {
		t.Errorf("branch = %q, want feature", branch)
	}
}

func TestNotARepo(t *testing.T) {
	dir := t.TempDir() // Empty dir, not a git repo
	g := NewGit(dir)

	_, err := g.CurrentBranch()
	// ZFC: Check for GitError with raw stderr for agent observation.
	// Agents decide what "not a git repository" means, not Go code.
	gitErr, ok := err.(*GitError)
	if !ok {
		t.Errorf("expected GitError, got %T: %v", err, err)
		return
	}
	// Verify raw stderr is available for agent observation
	if gitErr.Stderr == "" {
		t.Errorf("expected GitError with Stderr, got empty stderr")
	}
}

func TestRev(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)

	hash, err := g.Rev("HEAD")
	if err != nil {
		t.Fatalf("Rev: %v", err)
	}

	// Should be a 40-char hex string
	if len(hash) != 40 {
		t.Errorf("hash length = %d, want 40", len(hash))
	}
}

func TestFetchBranch(t *testing.T) {
	// Create a "remote" repo
	remoteDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	// Create a local repo and push to remote
	localDir := initTestRepo(t)
	g := NewGit(localDir)

	// Add remote
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git remote add: %v", err)
	}

	// Push main branch
	mainBranch, _ := g.CurrentBranch()
	cmd = exec.Command("git", "push", "-u", "origin", mainBranch)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git push: %v", err)
	}

	// Fetch should succeed
	if err := g.FetchBranch("origin", mainBranch); err != nil {
		t.Errorf("FetchBranch: %v", err)
	}
}

func TestCheckConflicts_NoConflict(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)
	mainBranch, _ := g.CurrentBranch()

	// Create feature branch with non-conflicting change
	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("feature"); err != nil {
		t.Fatalf("Checkout feature: %v", err)
	}

	// Add a new file (won't conflict with main)
	newFile := filepath.Join(dir, "feature.txt")
	if err := os.WriteFile(newFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := g.Add("feature.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("add feature file"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Go back to main
	if err := g.Checkout(mainBranch); err != nil {
		t.Fatalf("Checkout main: %v", err)
	}

	// Check for conflicts - should be none
	conflicts, err := g.CheckConflicts("feature", mainBranch)
	if err != nil {
		t.Fatalf("CheckConflicts: %v", err)
	}
	if len(conflicts) > 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	// Verify we're still on main and clean
	branch, _ := g.CurrentBranch()
	if branch != mainBranch {
		t.Errorf("branch = %q, want %q", branch, mainBranch)
	}
	status, _ := g.Status()
	if !status.Clean {
		t.Error("expected clean working directory after CheckConflicts")
	}
}

func TestCheckConflicts_WithConflict(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGit(dir)
	mainBranch, _ := g.CurrentBranch()

	// Create feature branch
	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("feature"); err != nil {
		t.Fatalf("Checkout feature: %v", err)
	}

	// Modify README.md on feature branch
	readmeFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Feature changes\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := g.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("modify readme on feature"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Go back to main and make conflicting change
	if err := g.Checkout(mainBranch); err != nil {
		t.Fatalf("Checkout main: %v", err)
	}
	if err := os.WriteFile(readmeFile, []byte("# Main changes\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := g.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("modify readme on main"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Check for conflicts - should find README.md
	conflicts, err := g.CheckConflicts("feature", mainBranch)
	if err != nil {
		t.Fatalf("CheckConflicts: %v", err)
	}
	if len(conflicts) == 0 {
		t.Error("expected conflicts, got none")
	}

	foundReadme := false
	for _, f := range conflicts {
		if f == "README.md" {
			foundReadme = true
			break
		}
	}
	if !foundReadme {
		t.Errorf("expected README.md in conflicts, got %v", conflicts)
	}

	// Verify we're still on main and clean
	branch, _ := g.CurrentBranch()
	if branch != mainBranch {
		t.Errorf("branch = %q, want %q", branch, mainBranch)
	}
	status, _ := g.Status()
	if !status.Clean {
		t.Error("expected clean working directory after CheckConflicts")
	}
}

// TestCloneBareHasOriginRefs verifies that after CloneBare, origin/* refs
// are available for worktree creation. This was broken before the fix:
// bare clones had refspec configured but no fetch was run, so origin/main
// didn't exist and WorktreeAddFromRef("origin/main") failed.
//
// Related: GitHub issue #286
func TestCloneBareHasOriginRefs(t *testing.T) {
	tmp := t.TempDir()

	// Create a "remote" repo with a commit on main
	remoteDir := filepath.Join(tmp, "remote")
	if err := os.MkdirAll(remoteDir, 0755); err != nil {
		t.Fatalf("mkdir remote: %v", err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = remoteDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = remoteDir
	_ = cmd.Run()

	// Create initial commit
	readmeFile := filepath.Join(remoteDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = remoteDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Get the main branch name (main or master depending on git version)
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = remoteDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	mainBranch := strings.TrimSpace(string(out))

	// Clone as bare repo using our CloneBare function
	bareDir := filepath.Join(tmp, "bare.git")
	g := NewGit(tmp)
	if err := g.CloneBare(remoteDir, bareDir); err != nil {
		t.Fatalf("CloneBare: %v", err)
	}

	// Verify origin/main exists (this was the bug - it didn't exist before the fix)
	bareGit := NewGitWithDir(bareDir, "")
	cmd = exec.Command("git", "--git-dir", bareDir, "branch", "-r")
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("git branch -r: %v", err)
	}

	originMain := "origin/" + mainBranch
	if !stringContains(string(out), originMain) {
		t.Errorf("expected %q in remote branches, got: %s", originMain, out)
	}

	// Verify WorktreeAddFromRef succeeds with origin/main
	// This is what polecat creation does
	worktreePath := filepath.Join(tmp, "worktree")
	if err := bareGit.WorktreeAddFromRef(worktreePath, "test-branch", originMain); err != nil {
		t.Errorf("WorktreeAddFromRef(%q) failed: %v", originMain, err)
	}

	// Verify the worktree was created and has the expected file
	worktreeReadme := filepath.Join(worktreePath, "README.md")
	if _, err := os.Stat(worktreeReadme); err != nil {
		t.Errorf("expected README.md in worktree: %v", err)
	}
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// initTestRepoWithRemote sets up a local repo with a bare remote and initial push.
// Returns (localDir, remoteDir, mainBranch).
func initTestRepoWithRemote(t *testing.T) (string, string, string) {
	t.Helper()
	tmp := t.TempDir()

	// Create bare remote
	remoteDir := filepath.Join(tmp, "remote.git")
	if err := exec.Command("git", "init", "--bare", remoteDir).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	// Create local repo
	localDir := filepath.Join(tmp, "local")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = localDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v", args, err)
		}
	}

	// Initial commit
	if err := os.WriteFile(filepath.Join(localDir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
		{"git", "remote", "add", "origin", remoteDir},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = localDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v", args, err)
		}
	}

	// Get main branch name and push
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = localDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("branch --show-current: %v", err)
	}
	mainBranch := strings.TrimSpace(string(out))

	cmd = exec.Command("git", "push", "-u", "origin", mainBranch)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("push: %v", err)
	}

	return localDir, remoteDir, mainBranch
}

func TestPruneStaleBranches_MergedBranch(t *testing.T) {
	localDir, _, mainBranch := initTestRepoWithRemote(t)
	g := NewGit(localDir)

	// Create a polecat branch, commit, and merge it to main
	if err := g.CreateBranch("polecat/test-merged"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("polecat/test-merged"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "feature.txt"), []byte("feature"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := g.Add("feature.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("add feature"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Push polecat branch to origin
	cmd := exec.Command("git", "push", "origin", "polecat/test-merged")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("push polecat branch: %v", err)
	}

	// Merge to main
	if err := g.Checkout(mainBranch); err != nil {
		t.Fatalf("Checkout main: %v", err)
	}
	if err := g.Merge("polecat/test-merged"); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Push main
	cmd = exec.Command("git", "push", "origin", mainBranch)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("push main: %v", err)
	}

	// Delete remote polecat branch (simulating refinery cleanup)
	cmd = exec.Command("git", "push", "origin", "--delete", "polecat/test-merged")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("delete remote branch: %v", err)
	}

	// Fetch --prune to remove remote tracking ref
	if err := g.FetchPrune("origin"); err != nil {
		t.Fatalf("FetchPrune: %v", err)
	}

	// Verify polecat branch still exists locally
	branches, err := g.ListBranches("polecat/*")
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 local polecat branch, got %d", len(branches))
	}

	// Prune should remove it
	pruned, err := g.PruneStaleBranches("polecat/*", false)
	if err != nil {
		t.Fatalf("PruneStaleBranches: %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("expected 1 pruned branch, got %d", len(pruned))
	}
	if pruned[0].Name != "polecat/test-merged" {
		t.Errorf("pruned name = %q, want polecat/test-merged", pruned[0].Name)
	}
	if pruned[0].Reason != "no-remote-merged" {
		t.Errorf("pruned reason = %q, want no-remote-merged", pruned[0].Reason)
	}

	// Verify branch is gone
	branches, err = g.ListBranches("polecat/*")
	if err != nil {
		t.Fatalf("ListBranches after prune: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected 0 branches after prune, got %d: %v", len(branches), branches)
	}
}

func TestPruneStaleBranches_DryRun(t *testing.T) {
	localDir, _, mainBranch := initTestRepoWithRemote(t)
	g := NewGit(localDir)

	// Create and merge a polecat branch (same as above)
	if err := g.CreateBranch("polecat/test-dryrun"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("polecat/test-dryrun"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "dry.txt"), []byte("dry"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := g.Add("dry.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("dry run test"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := g.Checkout(mainBranch); err != nil {
		t.Fatalf("Checkout main: %v", err)
	}
	if err := g.Merge("polecat/test-dryrun"); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Push main to update origin/main
	cmd := exec.Command("git", "push", "origin", mainBranch)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("push main: %v", err)
	}

	// Dry run should report but not delete
	pruned, err := g.PruneStaleBranches("polecat/*", true)
	if err != nil {
		t.Fatalf("PruneStaleBranches dry-run: %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("expected 1 branch in dry-run, got %d", len(pruned))
	}

	// Branch should still exist
	branches, err := g.ListBranches("polecat/*")
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Errorf("expected branch to still exist after dry-run, got %d branches", len(branches))
	}
}

func TestPruneStaleBranches_SkipsCurrentBranch(t *testing.T) {
	localDir, _, _ := initTestRepoWithRemote(t)
	g := NewGit(localDir)

	// Create and checkout a polecat branch (making it the current branch)
	if err := g.CreateBranch("polecat/current"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("polecat/current"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	// Prune should not delete the current branch
	pruned, err := g.PruneStaleBranches("polecat/*", false)
	if err != nil {
		t.Fatalf("PruneStaleBranches: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("expected 0 pruned (current branch should be skipped), got %d", len(pruned))
	}
}

func TestPruneStaleBranches_SkipsUnmerged(t *testing.T) {
	localDir, _, mainBranch := initTestRepoWithRemote(t)
	g := NewGit(localDir)

	// Create a polecat branch with a commit NOT merged to main
	if err := g.CreateBranch("polecat/unmerged"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.Checkout("polecat/unmerged"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "unmerged.txt"), []byte("unmerged"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := g.Add("unmerged.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit("unmerged work"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Push to remote so it has a remote tracking branch
	cmd := exec.Command("git", "push", "origin", "polecat/unmerged")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("push: %v", err)
	}

	if err := g.Checkout(mainBranch); err != nil {
		t.Fatalf("Checkout main: %v", err)
	}

	// Prune should NOT delete unmerged branch that still has remote
	pruned, err := g.PruneStaleBranches("polecat/*", false)
	if err != nil {
		t.Fatalf("PruneStaleBranches: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("expected 0 pruned (unmerged with remote should be kept), got %d", len(pruned))
	}
}

func TestFetchPrune(t *testing.T) {
	localDir, _, mainBranch := initTestRepoWithRemote(t)
	g := NewGit(localDir)

	// Create and push a branch
	if err := g.CreateBranch("polecat/prune-test"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	cmd := exec.Command("git", "push", "origin", "polecat/prune-test")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := g.Checkout(mainBranch); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	// Verify remote tracking ref exists
	exists, err := g.RemoteTrackingBranchExists("origin", "polecat/prune-test")
	if err != nil {
		t.Fatalf("RemoteTrackingBranchExists: %v", err)
	}
	if !exists {
		t.Fatal("expected remote tracking branch to exist")
	}

	// Delete remote branch
	cmd = exec.Command("git", "push", "origin", "--delete", "polecat/prune-test")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("delete remote: %v", err)
	}

	// FetchPrune should remove the stale tracking ref
	if err := g.FetchPrune("origin"); err != nil {
		t.Fatalf("FetchPrune: %v", err)
	}

	exists, err = g.RemoteTrackingBranchExists("origin", "polecat/prune-test")
	if err != nil {
		t.Fatalf("RemoteTrackingBranchExists after prune: %v", err)
	}
	if exists {
		t.Error("expected remote tracking branch to be pruned")
	}
}
