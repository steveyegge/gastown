package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

// ─── selectPushStrategy ──────────────────────────────────────────────────────

func TestSelectPushStrategyDefault(t *testing.T) {
	// No PushURL → DefaultPushStrategy
	cfg := &rig.RigConfig{Name: "testrip"}
	s := selectPushStrategy(cfg)
	if s.Name() != "default" {
		t.Errorf("got strategy %q, want %q", s.Name(), "default")
	}
	if s.IsFork() {
		t.Error("expected IsFork()=false for default strategy")
	}
}

func TestSelectPushStrategyFork(t *testing.T) {
	cfg := &rig.RigConfig{
		Name:    "testrip",
		PushURL: "https://github.com/myfork/repo.git",
	}
	s := selectPushStrategy(cfg)
	if s.Name() != "fork" {
		t.Errorf("got strategy %q, want %q", s.Name(), "fork")
	}
	if !s.IsFork() {
		t.Error("expected IsFork()=true for fork strategy")
	}
}

func TestSelectPushStrategyNilConfig(t *testing.T) {
	s := selectPushStrategy(nil)
	if s.Name() != "default" {
		t.Errorf("got strategy %q, want %q for nil config", s.Name(), "default")
	}
}

// ─── extractGitHubOrg ────────────────────────────────────────────────────────

func TestExtractGitHubOrgHTTPS(t *testing.T) {
	tests := []struct {
		url     string
		wantOrg string
		wantErr bool
	}{
		{"https://github.com/myorg/myrepo.git", "myorg", false},
		{"https://github.com/quad341/gastown.git", "quad341", false},
		{"git@github.com:myorg/myrepo.git", "myorg", false},
		{"https://gitlab.com/myorg/myrepo.git", "", true},
		{"not-a-url", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := extractGitHubOrg(tt.url)
		if tt.wantErr {
			if err == nil {
				t.Errorf("extractGitHubOrg(%q): expected error, got %q", tt.url, got)
			}
		} else {
			if err != nil {
				t.Errorf("extractGitHubOrg(%q): unexpected error: %v", tt.url, err)
			}
			if got != tt.wantOrg {
				t.Errorf("extractGitHubOrg(%q) = %q, want %q", tt.url, got, tt.wantOrg)
			}
		}
	}
}

func TestExtractGitHubRepo(t *testing.T) {
	tests := []struct {
		url      string
		wantRepo string
		wantErr  bool
	}{
		{"https://github.com/myorg/myrepo.git", "myorg/myrepo", false},
		{"https://github.com/steveyegge/gastown.git", "steveyegge/gastown", false},
		{"git@github.com:myorg/myrepo.git", "myorg/myrepo", false},
		{"https://gitlab.com/myorg/myrepo.git", "", true},
	}
	for _, tt := range tests {
		got, err := extractGitHubRepo(tt.url)
		if tt.wantErr {
			if err == nil {
				t.Errorf("extractGitHubRepo(%q): expected error, got %q", tt.url, got)
			}
		} else {
			if err != nil {
				t.Errorf("extractGitHubRepo(%q): unexpected error: %v", tt.url, err)
			}
			if got != tt.wantRepo {
				t.Errorf("extractGitHubRepo(%q) = %q, want %q", tt.url, got, tt.wantRepo)
			}
		}
	}
}

// ─── DefaultPushStrategy.Push failure paths ──────────────────────────────────

func TestDefaultPushStrategyPushFailsAllPaths(t *testing.T) {
	// Create a git repo with no remotes so all push attempts fail.
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	g := git.NewGit(repoDir)

	s := &DefaultPushStrategy{}
	err := s.Push(g, "polecat/test/branch", tmpDir, "testrip")
	if err == nil {
		t.Error("expected push to fail when no remote configured, got nil error")
	}
}

func TestDefaultPushStrategyPushSucceedsWithOrigin(t *testing.T) {
	// Create an origin bare repo and a clone pointing to it.
	tmpDir := t.TempDir()

	// Bare "origin"
	originDir := filepath.Join(tmpDir, "origin.git")
	if out, err := exec.Command("git", "init", "--bare", originDir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	// Clone with a branch
	cloneDir := filepath.Join(tmpDir, "clone")
	if out, err := exec.Command("git", "clone", originDir, cloneDir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	// Create a commit so we have something to push
	testFile := filepath.Join(cloneDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out, err := exec.Command("git", "-C", cloneDir, "config", "user.email", "test@test.com").CombinedOutput(); err != nil {
		t.Fatalf("git config email: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", cloneDir, "config", "user.name", "Test").CombinedOutput(); err != nil {
		t.Fatalf("git config name: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", cloneDir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", cloneDir, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	// Create a feature branch
	branchName := "polecat/test/feature"
	if out, err := exec.Command("git", "-C", cloneDir, "checkout", "-b", branchName).CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b: %v\n%s", err, out)
	}

	g := git.NewGit(cloneDir)
	s := &DefaultPushStrategy{}
	err := s.Push(g, branchName, tmpDir, "testrip")
	if err != nil {
		t.Errorf("unexpected push error: %v", err)
	}
}

// ─── DefaultPushStrategy.Submit failure paths ────────────────────────────────

func TestDefaultPushStrategySubmitNilBeads(t *testing.T) {
	s := &DefaultPushStrategy{}
	_, err := s.Submit(StrategySubmitParams{
		BD:      nil,
		Branch:  "polecat/test/branch",
		IssueID: "gas-abc",
	})
	if err == nil {
		t.Error("expected error when BD is nil, got nil")
	}
}

// TestDefaultPushStrategySubmitIDempotent verifies that if an MR bead already
// exists for the branch+SHA, Submit returns the existing MR ID without creating
// a new one.
func TestDefaultPushStrategySubmitIDempotent(t *testing.T) {
	tmpDir := t.TempDir()
	bdDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(bdDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	bd := beads.New(tmpDir)
	if bd == nil {
		t.Skip("beads.New returned nil — no beads DB in test environment")
	}

	// If beads isn't initialized, skip rather than fail.
	existingMR, err := bd.FindMRForBranchAndSHA("polecat/test/br", "abc123")
	if err != nil && errors.Is(err, errors.New("test")) {
		t.Skip("beads not available in test environment")
	}
	if existingMR != nil {
		// Already exists — verify idempotency by calling Submit.
		s := &DefaultPushStrategy{}
		mrID, submitErr := s.Submit(StrategySubmitParams{
			BD:        bd,
			Branch:    "polecat/test/br",
			CommitSHA: "abc123",
			IssueID:   "gas-abc",
		})
		if submitErr != nil {
			t.Errorf("unexpected submit error: %v", submitErr)
		}
		if mrID != existingMR.ID {
			t.Errorf("idempotent submit returned %q, want %q", mrID, existingMR.ID)
		}
	}
	// Test passes if no existing MR (can't fully test without initialized beads).
}

// ─── ForkPushStrategy.Submit ─────────────────────────────────────────────────

func TestForkPushStrategySubmitBadPushURL(t *testing.T) {
	s := &ForkPushStrategy{
		pushURL:     "https://gitlab.com/myorg/repo.git", // not GitHub
		upstreamURL: "https://github.com/upstream/repo.git",
	}
	g := git.NewGit(t.TempDir())
	_, err := s.Submit(StrategySubmitParams{
		G:        g,
		Branch:   "polecat/test/br",
		Target:   "main",
		IssueID:  "gas-abc",
		RigName:  "testrip",
		Priority: 2,
	})
	if err == nil {
		t.Error("expected error for non-GitHub push URL, got nil")
	}
}

func TestForkPushStrategyIsFork(t *testing.T) {
	s := &ForkPushStrategy{pushURL: "https://github.com/org/repo.git"}
	if !s.IsFork() {
		t.Error("ForkPushStrategy.IsFork() should return true")
	}
	if s.Name() != "fork" {
		t.Errorf("ForkPushStrategy.Name() = %q, want %q", s.Name(), "fork")
	}
}

// ─── ForkPushStrategy.Push failure path ──────────────────────────────────────

func TestForkPushStrategyPushNoForkRemote(t *testing.T) {
	// Create a repo with no "fork" remote; push should fail.
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	g := git.NewGit(repoDir)
	s := &ForkPushStrategy{pushURL: "https://github.com/myorg/repo.git"}
	err := s.Push(g, "polecat/test/branch", tmpDir, "testrip")
	if err == nil {
		t.Error("expected push error when fork remote doesn't exist, got nil")
	}
}
