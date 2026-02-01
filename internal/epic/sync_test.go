package epic

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepo provides helpers for creating test git repositories.
type TestRepo struct {
	Path string
}

// NewTestRepo creates a new git repository in a temp directory.
func NewTestRepo(t *testing.T, name string) *TestRepo {
	t.Helper()
	dir, err := os.MkdirTemp("", "epic-test-"+name+"-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	repo := &TestRepo{Path: dir}

	// Initialize repo
	if err := repo.run("init"); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to init repo: %v", err)
	}

	// Configure git for tests
	_ = repo.run("config", "user.email", "test@example.com")
	_ = repo.run("config", "user.name", "Test User")

	return repo
}

// Cleanup removes the test repository.
func (r *TestRepo) Cleanup() {
	os.RemoveAll(r.Path)
}

func (r *TestRepo) run(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &gitError{args: args, output: string(output), err: err}
	}
	return nil
}

func (r *TestRepo) runOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

type gitError struct {
	args   []string
	output string
	err    error
}

func (e *gitError) Error() string {
	return e.err.Error() + ": " + e.output
}

// CreateInitialCommit creates an initial commit with a README.
func (r *TestRepo) CreateInitialCommit(t *testing.T) string {
	t.Helper()
	readmePath := filepath.Join(r.Path, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	if err := r.run("add", "README.md"); err != nil {
		t.Fatalf("failed to add README: %v", err)
	}
	if err := r.run("commit", "-m", "Initial commit"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	sha, _ := r.runOutput("rev-parse", "HEAD")
	return sha
}

// CreateBranchWithCommit creates a branch and adds a commit with files.
func (r *TestRepo) CreateBranchWithCommit(t *testing.T, branch, base, message string, files map[string]string) string {
	t.Helper()
	if err := r.run("checkout", "-b", branch, base); err != nil {
		t.Fatalf("failed to create branch %s: %v", branch, err)
	}
	for name, content := range files {
		fullPath := filepath.Join(r.Path, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
		if err := r.run("add", name); err != nil {
			t.Fatalf("failed to add %s: %v", name, err)
		}
	}
	if err := r.run("commit", "-m", message); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	sha, _ := r.runOutput("rev-parse", "HEAD")
	return sha
}

// AddCommit adds a commit with the given files to the current branch.
func (r *TestRepo) AddCommit(t *testing.T, message string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		fullPath := filepath.Join(r.Path, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
		if err := r.run("add", name); err != nil {
			t.Fatalf("failed to add %s: %v", name, err)
		}
	}
	if err := r.run("commit", "-m", message); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	sha, _ := r.runOutput("rev-parse", "HEAD")
	return sha
}

// Checkout switches to the given branch.
func (r *TestRepo) Checkout(t *testing.T, branch string) {
	t.Helper()
	if err := r.run("checkout", branch); err != nil {
		t.Fatalf("failed to checkout %s: %v", branch, err)
	}
}

// GetMainBranch returns "main" or "master" depending on git config.
func (r *TestRepo) GetMainBranch(t *testing.T) string {
	t.Helper()
	// Try main first, fall back to master
	if err := r.run("rev-parse", "--verify", "main"); err == nil {
		return "main"
	}
	return "master"
}

// AddRemote adds a remote to the repository.
func (r *TestRepo) AddRemote(t *testing.T, name, url string) {
	t.Helper()
	if err := r.run("remote", "add", name, url); err != nil {
		t.Fatalf("failed to add remote %s: %v", name, err)
	}
}

// NewBareRepo creates a bare git repository (for simulating remotes).
func NewBareRepo(t *testing.T, name string) *TestRepo {
	t.Helper()
	dir, err := os.MkdirTemp("", "epic-bare-"+name+"-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	repo := &TestRepo{Path: dir}
	if err := repo.run("init", "--bare"); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to init bare repo: %v", err)
	}

	return repo
}

// === Tests for sync.go ===

func TestFetchUpstream(t *testing.T) {
	// Create a "remote" repo
	remote := NewBareRepo(t, "remote")
	defer remote.Cleanup()

	// Create local repo
	local := NewTestRepo(t, "local")
	defer local.Cleanup()

	// Initialize local with a commit and push to remote
	local.CreateInitialCommit(t)
	mainBranch := local.GetMainBranch(t)
	local.AddRemote(t, "origin", remote.Path)
	if err := local.run("push", "-u", "origin", mainBranch); err != nil {
		t.Fatalf("failed to push: %v", err)
	}

	// Test FetchUpstream
	err := FetchUpstream(local.Path, "origin")
	if err != nil {
		t.Errorf("FetchUpstream failed: %v", err)
	}
}

func TestFetchUpstream_InvalidRemote(t *testing.T) {
	repo := NewTestRepo(t, "no-remote")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)

	err := FetchUpstream(repo.Path, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent remote")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	repo := NewTestRepo(t, "current-branch")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)

	branch, err := getCurrentBranch(repo.Path)
	if err != nil {
		t.Fatalf("getCurrentBranch failed: %v", err)
	}

	mainBranch := repo.GetMainBranch(t)
	if branch != mainBranch {
		t.Errorf("expected %s, got %s", mainBranch, branch)
	}

	// Create and switch to a new branch
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"feature.txt": "feature content",
	})

	branch, err = getCurrentBranch(repo.Path)
	if err != nil {
		t.Fatalf("getCurrentBranch failed: %v", err)
	}
	if branch != "feature" {
		t.Errorf("expected feature, got %s", branch)
	}
}

func TestCountCommits(t *testing.T) {
	repo := NewTestRepo(t, "count-commits")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)
	mainBranch := repo.GetMainBranch(t)

	// Create a branch with 3 commits
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "commit 1", map[string]string{
		"a.txt": "a",
	})
	repo.AddCommit(t, "commit 2", map[string]string{
		"b.txt": "b",
	})
	repo.AddCommit(t, "commit 3", map[string]string{
		"c.txt": "c",
	})

	count, err := countCommits(repo.Path, mainBranch, "feature")
	if err != nil {
		t.Fatalf("countCommits failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 commits, got %d", count)
	}
}

func TestCheckConflicts_NoConflict(t *testing.T) {
	repo := NewTestRepo(t, "no-conflict")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)
	mainBranch := repo.GetMainBranch(t)

	// Create a branch with changes to a different file
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"feature.txt": "feature content",
	})

	conflict, err := CheckConflicts(repo.Path, "feature", mainBranch)
	if err != nil {
		t.Fatalf("CheckConflicts failed: %v", err)
	}
	if conflict != nil {
		t.Errorf("expected no conflict, got %+v", conflict)
	}
}

func TestCheckConflicts_WithConflict(t *testing.T) {
	repo := NewTestRepo(t, "with-conflict")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)
	mainBranch := repo.GetMainBranch(t)

	// Create a branch with changes to README
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"README.md": "feature content",
	})

	// Go back to main and make conflicting changes
	repo.Checkout(t, mainBranch)
	repo.AddCommit(t, "main commit", map[string]string{
		"README.md": "main content",
	})

	conflict, err := CheckConflicts(repo.Path, "feature", mainBranch)
	if err != nil {
		t.Fatalf("CheckConflicts failed: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected conflict, got nil")
	}
	if conflict.Branch != "feature" {
		t.Errorf("expected branch 'feature', got %s", conflict.Branch)
	}
	if conflict.BaseBranch != mainBranch {
		t.Errorf("expected base branch %s, got %s", mainBranch, conflict.BaseBranch)
	}
}

func TestRebaseBranch_Success(t *testing.T) {
	repo := NewTestRepo(t, "rebase-success")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)
	mainBranch := repo.GetMainBranch(t)

	// Create a feature branch
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"feature.txt": "feature content",
	})

	// Go back to main and add a non-conflicting commit
	repo.Checkout(t, mainBranch)
	repo.AddCommit(t, "main advance", map[string]string{
		"main.txt": "main content",
	})

	// Rebase feature onto main
	result, err := RebaseBranch(repo.Path, "feature", mainBranch)
	if err != nil {
		t.Fatalf("RebaseBranch failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Message)
	}
	if result.CommitCount != 1 {
		t.Errorf("expected 1 commit rebased, got %d", result.CommitCount)
	}
}

func TestRebaseBranch_Conflict(t *testing.T) {
	repo := NewTestRepo(t, "rebase-conflict")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)
	mainBranch := repo.GetMainBranch(t)

	// Create a feature branch that modifies README
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"README.md": "feature content",
	})

	// Go back to main and make conflicting changes
	repo.Checkout(t, mainBranch)
	repo.AddCommit(t, "main commit", map[string]string{
		"README.md": "main content",
	})

	// Try to rebase - should fail with conflict
	result, err := RebaseBranch(repo.Path, "feature", mainBranch)
	if err != nil {
		t.Fatalf("RebaseBranch returned error: %v", err)
	}
	if result.Success {
		t.Error("expected failure due to conflict")
	}
	if result.Conflicts == nil {
		t.Error("expected conflict info")
	}
}

func TestForcePushBranch(t *testing.T) {
	// Create a "remote" repo
	remote := NewBareRepo(t, "push-remote")
	defer remote.Cleanup()

	// Create local repo
	local := NewTestRepo(t, "push-local")
	defer local.Cleanup()

	// Initialize and push
	local.CreateInitialCommit(t)
	mainBranch := local.GetMainBranch(t)
	local.AddRemote(t, "origin", remote.Path)
	if err := local.run("push", "-u", "origin", mainBranch); err != nil {
		t.Fatalf("failed to push: %v", err)
	}

	// Create a feature branch and push it
	local.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"feature.txt": "feature content",
	})
	if err := local.run("push", "-u", "origin", "feature"); err != nil {
		t.Fatalf("failed to push feature: %v", err)
	}

	// Amend the commit (simulating rebase)
	local.AddCommit(t, "amended feature", map[string]string{
		"feature2.txt": "more feature",
	})

	// Force push should succeed
	err := ForcePushBranch(local.Path, "origin", "feature")
	if err != nil {
		t.Errorf("ForcePushBranch failed: %v", err)
	}
}

func TestGetConflictingFiles(t *testing.T) {
	repo := NewTestRepo(t, "conflict-files")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)
	mainBranch := repo.GetMainBranch(t)

	// Create conflicting branches
	repo.CreateBranchWithCommit(t, "feature", mainBranch, "feature commit", map[string]string{
		"conflict.txt": "feature version",
		"a.txt":        "feature a",
	})

	repo.Checkout(t, mainBranch)
	repo.AddCommit(t, "main commit", map[string]string{
		"conflict.txt": "main version",
		"a.txt":        "main a",
	})

	// Try to merge to create conflict state
	repo.Checkout(t, "feature")
	mergeCmd := exec.Command("git", "merge", "--no-commit", mainBranch)
	mergeCmd.Dir = repo.Path
	_ = mergeCmd.Run() // This will fail due to conflicts

	// Get conflict files
	files, err := getConflictingFiles(repo.Path)
	if err != nil {
		t.Fatalf("getConflictingFiles failed: %v", err)
	}

	// Clean up merge state
	_ = repo.run("merge", "--abort")

	// Should have 2 conflicting files
	if len(files) != 2 {
		t.Errorf("expected 2 conflicting files, got %d: %v", len(files), files)
	}
}

// === Tests for helper functions ===

func TestSyncResult_Message(t *testing.T) {
	result := &SyncResult{
		Branch:  "feature",
		Success: true,
		Message: "Successfully synced",
	}

	if result.Message != "Successfully synced" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRebaseResult_WithConflicts(t *testing.T) {
	result := &RebaseResult{
		Branch:     "feature",
		BaseBranch: "main",
		Success:    false,
		Conflicts: &ConflictInfo{
			Branch:     "feature",
			BaseBranch: "main",
			Files:      []string{"README.md", "config.go"},
		},
		Message: "Rebase failed with conflicts in 2 file(s)",
	}

	if result.Success {
		t.Error("expected failure")
	}
	if len(result.Conflicts.Files) != 2 {
		t.Errorf("expected 2 conflict files, got %d", len(result.Conflicts.Files))
	}
}

func TestConflictInfo_Fields(t *testing.T) {
	info := &ConflictInfo{
		Branch:     "feature",
		BaseBranch: "main",
		Files:      []string{"a.txt", "b.txt"},
		PRNumber:   123,
	}

	if info.Branch != "feature" {
		t.Errorf("unexpected branch: %s", info.Branch)
	}
	if info.PRNumber != 123 {
		t.Errorf("unexpected PR number: %d", info.PRNumber)
	}
}

func TestCIStatus_States(t *testing.T) {
	tests := []struct {
		state    string
		expected string
	}{
		{"success", "success"},
		{"failure", "failure"},
		{"pending", "pending"},
		{"error", "error"},
	}

	for _, tt := range tests {
		status := &CIStatus{
			PRNumber: 123,
			State:    tt.state,
		}
		if status.State != tt.expected {
			t.Errorf("expected state %s, got %s", tt.expected, status.State)
		}
	}
}

// === Tests for gh CLI functions (require gh to be installed) ===

func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func TestGetPRCIStatus_InvalidPR(t *testing.T) {
	if !ghAvailable() {
		t.Skip("gh CLI not available")
	}

	// Use a temp directory as workDir
	repo := NewTestRepo(t, "ci-status")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)

	// This should fail because there's no GitHub repo configured
	_, err := GetPRCIStatus(repo.Path, 99999)
	if err == nil {
		t.Error("expected error for invalid PR/repo")
	}
}

func TestGetPRReviewStatus_InvalidPR(t *testing.T) {
	if !ghAvailable() {
		t.Skip("gh CLI not available")
	}

	repo := NewTestRepo(t, "review-status")
	defer repo.Cleanup()
	repo.CreateInitialCommit(t)

	// This should fail because there's no GitHub repo configured
	_, _, err := GetPRReviewStatus(repo.Path, 99999)
	if err == nil {
		t.Error("expected error for invalid PR/repo")
	}
}

// TestCIStatus_Aggregation tests the CI status aggregation logic.
func TestCIStatus_Aggregation(t *testing.T) {
	// Test the aggregation logic without calling gh
	// This validates the structure and fields

	status := &CIStatus{
		PRNumber: 123,
		State:    "success",
		Details:  "All checks passed",
		URL:      "https://github.com/owner/repo/pull/123/checks",
	}

	if status.PRNumber != 123 {
		t.Errorf("expected PR 123, got %d", status.PRNumber)
	}
	if status.State != "success" {
		t.Errorf("expected success, got %s", status.State)
	}
	if status.Details != "All checks passed" {
		t.Errorf("expected 'All checks passed', got %s", status.Details)
	}
	if status.URL == "" {
		t.Error("expected non-empty URL")
	}

	// Test failure state
	failedStatus := &CIStatus{
		PRNumber: 456,
		State:    "failure",
		Details:  "Failed: lint, test",
		URL:      "https://github.com/owner/repo/pull/456/checks",
	}

	if failedStatus.State != "failure" {
		t.Errorf("expected failure, got %s", failedStatus.State)
	}

	// Test pending state
	pendingStatus := &CIStatus{
		PRNumber: 789,
		State:    "pending",
		Details:  "Pending: build",
	}

	if pendingStatus.State != "pending" {
		t.Errorf("expected pending, got %s", pendingStatus.State)
	}
}

// TestPRStatusConstants tests the PR status constants.
func TestPRStatusConstants(t *testing.T) {
	if PRStatusOpen != "open" {
		t.Errorf("PRStatusOpen: expected 'open', got '%s'", PRStatusOpen)
	}
	if PRStatusMerged != "merged" {
		t.Errorf("PRStatusMerged: expected 'merged', got '%s'", PRStatusMerged)
	}
	if PRStatusClosed != "closed" {
		t.Errorf("PRStatusClosed: expected 'closed', got '%s'", PRStatusClosed)
	}
	if PRStatusChangesRequested != "changes_requested" {
		t.Errorf("PRStatusChangesRequested: expected 'changes_requested', got '%s'", PRStatusChangesRequested)
	}
	if PRStatusApproved != "approved" {
		t.Errorf("PRStatusApproved: expected 'approved', got '%s'", PRStatusApproved)
	}
	if PRStatusDraft != "draft" {
		t.Errorf("PRStatusDraft: expected 'draft', got '%s'", PRStatusDraft)
	}
}

// === Tests using StubGHClient ===

func TestGetPRCIStatusWithClient_AllPassing(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "SUCCESS", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "SUCCESS", Name: "test", DetailsURL: "https://example.com/test"},
			{State: "SUCCESS", Name: "build", DetailsURL: "https://example.com/build"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "success" {
		t.Errorf("expected state 'success', got '%s'", status.State)
	}
	if status.PRNumber != 123 {
		t.Errorf("expected PR number 123, got %d", status.PRNumber)
	}
	if status.Details != "All checks passed" {
		t.Errorf("expected 'All checks passed', got '%s'", status.Details)
	}

	// Verify the client was called correctly
	if len(stub.CallLog) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.CallLog))
	}
	if stub.CallLog[0].Method != "GetPRChecks" {
		t.Errorf("expected GetPRChecks, got %s", stub.CallLog[0].Method)
	}
	if stub.CallLog[0].PRNumber != 123 {
		t.Errorf("expected PR 123, got %d", stub.CallLog[0].PRNumber)
	}
}

func TestGetPRCIStatusWithClient_WithFailure(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "SUCCESS", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "FAILURE", Name: "test", DetailsURL: "https://example.com/test"},
			{State: "SUCCESS", Name: "build", DetailsURL: "https://example.com/build"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "failure" {
		t.Errorf("expected state 'failure', got '%s'", status.State)
	}
	if status.Details != "Failed: test" {
		t.Errorf("expected 'Failed: test', got '%s'", status.Details)
	}
	if status.URL != "https://example.com/test" {
		t.Errorf("expected URL to first failed check, got '%s'", status.URL)
	}
}

func TestGetPRCIStatusWithClient_WithMultipleFailures(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "FAILURE", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "FAILURE", Name: "test", DetailsURL: "https://example.com/test"},
			{State: "SUCCESS", Name: "build", DetailsURL: "https://example.com/build"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 789)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "failure" {
		t.Errorf("expected state 'failure', got '%s'", status.State)
	}
	if status.Details != "Failed: lint, test" {
		t.Errorf("expected 'Failed: lint, test', got '%s'", status.Details)
	}
	// URL should be from first failed check
	if status.URL != "https://example.com/lint" {
		t.Errorf("expected URL to first failed check, got '%s'", status.URL)
	}
}

func TestGetPRCIStatusWithClient_WithPending(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "SUCCESS", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "PENDING", Name: "test", DetailsURL: "https://example.com/test"},
			{State: "IN_PROGRESS", Name: "build", DetailsURL: "https://example.com/build"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 111)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.State != "pending" {
		t.Errorf("expected state 'pending', got '%s'", status.State)
	}
	if status.Details != "Pending: test, build" {
		t.Errorf("expected 'Pending: test, build', got '%s'", status.Details)
	}
}

func TestGetPRCIStatusWithClient_FailureTakesPrecedenceOverPending(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "SUCCESS", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "PENDING", Name: "test", DetailsURL: "https://example.com/test"},
			{State: "FAILURE", Name: "build", DetailsURL: "https://example.com/build"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 222)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Failure should take precedence over pending
	if status.State != "failure" {
		t.Errorf("expected state 'failure', got '%s'", status.State)
	}
}

func TestGetPRCIStatusWithClient_ErrorState(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "SUCCESS", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "ERROR", Name: "test", DetailsURL: "https://example.com/test"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 333)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ERROR should be treated as failure
	if status.State != "failure" {
		t.Errorf("expected state 'failure', got '%s'", status.State)
	}
}

func TestGetPRCIStatusWithClient_QueuedState(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{
			{State: "SUCCESS", Name: "lint", DetailsURL: "https://example.com/lint"},
			{State: "QUEUED", Name: "test", DetailsURL: "https://example.com/test"},
		},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 444)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// QUEUED should be treated as pending
	if status.State != "pending" {
		t.Errorf("expected state 'pending', got '%s'", status.State)
	}
}

func TestGetPRCIStatusWithClient_Error(t *testing.T) {
	stub := &StubGHClient{
		ChecksError: &gitError{args: []string{"gh", "pr", "checks"}, output: "not found", err: nil},
	}

	_, err := GetPRCIStatusWithClient(stub, "/fake/dir", 555)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetPRCIStatusWithClient_EmptyChecks(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{},
	}

	status, err := GetPRCIStatusWithClient(stub, "/fake/dir", 666)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No checks should result in success
	if status.State != "success" {
		t.Errorf("expected state 'success', got '%s'", status.State)
	}
	if status.Details != "All checks passed" {
		t.Errorf("expected 'All checks passed', got '%s'", status.Details)
	}
}

func TestGetPRReviewStatusWithClient_Approved(t *testing.T) {
	stub := &StubGHClient{
		ReviewsResponse: &PRReviewInfo{
			ReviewDecision: "APPROVED",
			Reviews: []PRReview{
				{State: "APPROVED"},
				{State: "APPROVED"},
			},
		},
	}

	status, approvals, err := GetPRReviewStatusWithClient(stub, "/fake/dir", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status != PRStatusApproved {
		t.Errorf("expected status '%s', got '%s'", PRStatusApproved, status)
	}
	if approvals != 2 {
		t.Errorf("expected 2 approvals, got %d", approvals)
	}

	// Verify the client was called correctly
	if len(stub.CallLog) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.CallLog))
	}
	if stub.CallLog[0].Method != "GetPRReviews" {
		t.Errorf("expected GetPRReviews, got %s", stub.CallLog[0].Method)
	}
}

func TestGetPRReviewStatusWithClient_ChangesRequested(t *testing.T) {
	stub := &StubGHClient{
		ReviewsResponse: &PRReviewInfo{
			ReviewDecision: "CHANGES_REQUESTED",
			Reviews: []PRReview{
				{State: "CHANGES_REQUESTED"},
			},
		},
	}

	status, approvals, err := GetPRReviewStatusWithClient(stub, "/fake/dir", 456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status != PRStatusChangesRequested {
		t.Errorf("expected status '%s', got '%s'", PRStatusChangesRequested, status)
	}
	if approvals != 0 {
		t.Errorf("expected 0 approvals, got %d", approvals)
	}
}

func TestGetPRReviewStatusWithClient_ReviewRequired(t *testing.T) {
	stub := &StubGHClient{
		ReviewsResponse: &PRReviewInfo{
			ReviewDecision: "REVIEW_REQUIRED",
			Reviews:        []PRReview{},
		},
	}

	status, approvals, err := GetPRReviewStatusWithClient(stub, "/fake/dir", 789)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status != "review_required" {
		t.Errorf("expected status 'review_required', got '%s'", status)
	}
	if approvals != 0 {
		t.Errorf("expected 0 approvals, got %d", approvals)
	}
}

func TestGetPRReviewStatusWithClient_Pending(t *testing.T) {
	stub := &StubGHClient{
		ReviewsResponse: &PRReviewInfo{
			ReviewDecision: "", // Empty means pending
			Reviews:        []PRReview{},
		},
	}

	status, approvals, err := GetPRReviewStatusWithClient(stub, "/fake/dir", 111)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", status)
	}
	if approvals != 0 {
		t.Errorf("expected 0 approvals, got %d", approvals)
	}
}

func TestGetPRReviewStatusWithClient_MixedReviews(t *testing.T) {
	stub := &StubGHClient{
		ReviewsResponse: &PRReviewInfo{
			ReviewDecision: "APPROVED",
			Reviews: []PRReview{
				{State: "APPROVED"},
				{State: "COMMENTED"},
				{State: "APPROVED"},
				{State: "DISMISSED"},
			},
		},
	}

	status, approvals, err := GetPRReviewStatusWithClient(stub, "/fake/dir", 222)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status != PRStatusApproved {
		t.Errorf("expected status '%s', got '%s'", PRStatusApproved, status)
	}
	// Only APPROVED states should be counted
	if approvals != 2 {
		t.Errorf("expected 2 approvals, got %d", approvals)
	}
}

func TestGetPRReviewStatusWithClient_Error(t *testing.T) {
	stub := &StubGHClient{
		ReviewsError: &gitError{args: []string{"gh", "pr", "view"}, output: "not found", err: nil},
	}

	_, _, err := GetPRReviewStatusWithClient(stub, "/fake/dir", 333)
	if err == nil {
		t.Error("expected error")
	}
}

func TestStubGHClient_CallLogTracking(t *testing.T) {
	stub := &StubGHClient{
		ChecksResponse: []PRCheck{},
		ReviewsResponse: &PRReviewInfo{
			ReviewDecision: "APPROVED",
			Reviews:        []PRReview{{State: "APPROVED"}},
		},
	}

	// Make multiple calls
	_, _ = GetPRCIStatusWithClient(stub, "/dir1", 100)
	_, _ = GetPRCIStatusWithClient(stub, "/dir2", 200)
	_, _, _ = GetPRReviewStatusWithClient(stub, "/dir3", 300)

	// Verify call log
	if len(stub.CallLog) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(stub.CallLog))
	}

	// First call
	if stub.CallLog[0].Method != "GetPRChecks" || stub.CallLog[0].WorkDir != "/dir1" || stub.CallLog[0].PRNumber != 100 {
		t.Errorf("unexpected first call: %+v", stub.CallLog[0])
	}

	// Second call
	if stub.CallLog[1].Method != "GetPRChecks" || stub.CallLog[1].WorkDir != "/dir2" || stub.CallLog[1].PRNumber != 200 {
		t.Errorf("unexpected second call: %+v", stub.CallLog[1])
	}

	// Third call
	if stub.CallLog[2].Method != "GetPRReviews" || stub.CallLog[2].WorkDir != "/dir3" || stub.CallLog[2].PRNumber != 300 {
		t.Errorf("unexpected third call: %+v", stub.CallLog[2])
	}
}

func TestAggregateCIStatus_DirectCall(t *testing.T) {
	// Test the aggregateCIStatus function directly
	tests := []struct {
		name           string
		prNumber       int
		checks         []PRCheck
		expectedState  string
		expectedDetail string
	}{
		{
			name:           "empty checks",
			prNumber:       1,
			checks:         []PRCheck{},
			expectedState:  "success",
			expectedDetail: "All checks passed",
		},
		{
			name:     "all success",
			prNumber: 2,
			checks: []PRCheck{
				{State: "SUCCESS", Name: "a"},
				{State: "SUCCESS", Name: "b"},
			},
			expectedState:  "success",
			expectedDetail: "All checks passed",
		},
		{
			name:     "one failure",
			prNumber: 3,
			checks: []PRCheck{
				{State: "SUCCESS", Name: "a"},
				{State: "FAILURE", Name: "b"},
			},
			expectedState:  "failure",
			expectedDetail: "Failed: b",
		},
		{
			name:     "one pending",
			prNumber: 4,
			checks: []PRCheck{
				{State: "SUCCESS", Name: "a"},
				{State: "PENDING", Name: "b"},
			},
			expectedState:  "pending",
			expectedDetail: "Pending: b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aggregateCIStatus(tt.prNumber, tt.checks)
			if result.State != tt.expectedState {
				t.Errorf("expected state %s, got %s", tt.expectedState, result.State)
			}
			if result.Details != tt.expectedDetail {
				t.Errorf("expected detail '%s', got '%s'", tt.expectedDetail, result.Details)
			}
			if result.PRNumber != tt.prNumber {
				t.Errorf("expected PR %d, got %d", tt.prNumber, result.PRNumber)
			}
		})
	}
}

func TestParseReviewStatus_DirectCall(t *testing.T) {
	// Test the parseReviewStatus function directly
	tests := []struct {
		name              string
		info              *PRReviewInfo
		expectedStatus    string
		expectedApprovals int
	}{
		{
			name:              "approved with two approvals",
			info:              &PRReviewInfo{ReviewDecision: "APPROVED", Reviews: []PRReview{{State: "APPROVED"}, {State: "APPROVED"}}},
			expectedStatus:    PRStatusApproved,
			expectedApprovals: 2,
		},
		{
			name:              "changes requested",
			info:              &PRReviewInfo{ReviewDecision: "CHANGES_REQUESTED", Reviews: []PRReview{{State: "CHANGES_REQUESTED"}}},
			expectedStatus:    PRStatusChangesRequested,
			expectedApprovals: 0,
		},
		{
			name:              "review required",
			info:              &PRReviewInfo{ReviewDecision: "REVIEW_REQUIRED", Reviews: []PRReview{}},
			expectedStatus:    "review_required",
			expectedApprovals: 0,
		},
		{
			name:              "pending (empty decision)",
			info:              &PRReviewInfo{ReviewDecision: "", Reviews: []PRReview{}},
			expectedStatus:    "pending",
			expectedApprovals: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, approvals, err := parseReviewStatus(tt.info)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, status)
			}
			if approvals != tt.expectedApprovals {
				t.Errorf("expected %d approvals, got %d", tt.expectedApprovals, approvals)
			}
		})
	}
}
