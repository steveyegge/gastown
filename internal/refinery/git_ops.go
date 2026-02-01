// Package refinery provides the merge queue processing agent.
package refinery

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitOperations defines the interface for git operations.
// This allows mocking in tests while using real git in production.
type GitOperations interface {
	// Branch operations
	GetCurrentBranch() (string, error)
	BranchExists(name string) bool
	CreateBranch(name, base string) error
	DeleteBranch(name string) error
	CheckoutBranch(name string) error

	// Commit operations
	GetHeadSHA(branch string) (string, error)
	GetMergeBase(branch1, branch2 string) (string, error)
	AddCommit(message string, files map[string]string) (string, error)

	// Rebase operations
	CanRebase(branch, onto string) (bool, []string, error) // returns canRebase, conflictFiles, error
	Rebase(branch, onto string) error
	AbortRebase() error

	// Merge operations
	CanMerge(branch, into string) (bool, []string, error)
	Merge(branch, into string, squash bool) error
	AbortMerge() error

	// Remote operations
	Fetch(remote string) error
	Push(remote, branch string, force bool) error
	Pull(remote, branch string) error

	// Status
	HasUncommittedChanges() (bool, error)
	GetStatus() (string, error)
}

// RealGitOps implements GitOperations using actual git commands.
type RealGitOps struct {
	WorkDir string
}

// NewRealGitOps creates a new RealGitOps for the given directory.
func NewRealGitOps(workDir string) *RealGitOps {
	return &RealGitOps{WorkDir: workDir}
}

func (g *RealGitOps) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.WorkDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (g *RealGitOps) runAllowFailure(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.WorkDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // ignore error
	return strings.TrimSpace(stdout.String()), nil
}

func (g *RealGitOps) GetCurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

func (g *RealGitOps) BranchExists(name string) bool {
	_, err := g.run("rev-parse", "--verify", name)
	return err == nil
}

func (g *RealGitOps) CreateBranch(name, base string) error {
	_, err := g.run("checkout", "-b", name, base)
	return err
}

func (g *RealGitOps) DeleteBranch(name string) error {
	_, err := g.run("branch", "-D", name)
	return err
}

func (g *RealGitOps) CheckoutBranch(name string) error {
	_, err := g.run("checkout", name)
	return err
}

func (g *RealGitOps) GetHeadSHA(branch string) (string, error) {
	return g.run("rev-parse", branch)
}

func (g *RealGitOps) GetMergeBase(branch1, branch2 string) (string, error) {
	return g.run("merge-base", branch1, branch2)
}

func (g *RealGitOps) AddCommit(message string, files map[string]string) (string, error) {
	for path, content := range files {
		fullPath := filepath.Join(g.WorkDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("creating directory %s: %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("writing file %s: %w", path, err)
		}
		if _, err := g.run("add", path); err != nil {
			return "", err
		}
	}
	if _, err := g.run("commit", "-m", message); err != nil {
		return "", err
	}
	return g.GetHeadSHA("HEAD")
}

func (g *RealGitOps) CanRebase(branch, onto string) (bool, []string, error) {
	// Save current branch
	currentBranch, err := g.GetCurrentBranch()
	if err != nil {
		return false, nil, err
	}

	// Checkout the branch to rebase
	if err := g.CheckoutBranch(branch); err != nil {
		return false, nil, err
	}

	// Try the rebase
	_, err = g.run("rebase", "--no-commit", onto)
	if err != nil {
		// Get conflict files
		conflictFiles, _ := g.getConflictFiles()
		_ = g.AbortRebase()
		_ = g.CheckoutBranch(currentBranch)
		return false, conflictFiles, nil
	}

	// Abort the successful rebase (we just wanted to check)
	_ = g.AbortRebase()
	_ = g.CheckoutBranch(currentBranch)
	return true, nil, nil
}

func (g *RealGitOps) getConflictFiles() ([]string, error) {
	output, err := g.runAllowFailure("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	if output == "" {
		return nil, nil
	}
	return strings.Split(output, "\n"), nil
}

func (g *RealGitOps) Rebase(branch, onto string) error {
	if err := g.CheckoutBranch(branch); err != nil {
		return err
	}
	_, err := g.run("rebase", onto)
	return err
}

func (g *RealGitOps) AbortRebase() error {
	_, err := g.runAllowFailure("rebase", "--abort")
	return err
}

func (g *RealGitOps) CanMerge(branch, into string) (bool, []string, error) {
	// Save current branch
	currentBranch, err := g.GetCurrentBranch()
	if err != nil {
		return false, nil, err
	}

	// Checkout target branch
	if err := g.CheckoutBranch(into); err != nil {
		return false, nil, err
	}

	// Try merge with no commit
	_, err = g.run("merge", "--no-commit", "--no-ff", branch)
	if err != nil {
		conflictFiles, _ := g.getConflictFiles()
		_ = g.AbortMerge()
		_ = g.CheckoutBranch(currentBranch)
		return false, conflictFiles, nil
	}

	// Abort the successful merge
	_ = g.AbortMerge()
	_ = g.CheckoutBranch(currentBranch)
	return true, nil, nil
}

func (g *RealGitOps) Merge(branch, into string, squash bool) error {
	if err := g.CheckoutBranch(into); err != nil {
		return err
	}
	args := []string{"merge"}
	if squash {
		args = append(args, "--squash")
	}
	args = append(args, branch)
	_, err := g.run(args...)
	if squash && err == nil {
		// Squash merge needs a commit
		_, err = g.run("commit", "-m", fmt.Sprintf("Merge %s (squashed)", branch))
	}
	return err
}

func (g *RealGitOps) AbortMerge() error {
	_, err := g.runAllowFailure("merge", "--abort")
	return err
}

func (g *RealGitOps) Fetch(remote string) error {
	_, err := g.run("fetch", remote)
	return err
}

func (g *RealGitOps) Push(remote, branch string, force bool) error {
	args := []string{"push", remote, branch}
	if force {
		args = []string{"push", "--force-with-lease", remote, branch}
	}
	_, err := g.run(args...)
	return err
}

func (g *RealGitOps) Pull(remote, branch string) error {
	_, err := g.run("pull", remote, branch)
	return err
}

func (g *RealGitOps) HasUncommittedChanges() (bool, error) {
	output, err := g.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return output != "", nil
}

func (g *RealGitOps) GetStatus() (string, error) {
	return g.run("status", "--short")
}

// TestRepo provides helpers for creating test git repositories.
type TestRepo struct {
	Path string
	Git  *RealGitOps
}

// NewTestRepo creates a new git repository in a temp directory.
func NewTestRepo(name string) (*TestRepo, error) {
	dir, err := os.MkdirTemp("", "gastown-test-"+name+"-*")
	if err != nil {
		return nil, err
	}

	git := NewRealGitOps(dir)

	// Initialize repo with main as default branch
	if _, err := git.run("init", "-b", "main"); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	// Configure git for tests
	if _, err := git.run("config", "user.email", "test@example.com"); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if _, err := git.run("config", "user.name", "Test User"); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	return &TestRepo{Path: dir, Git: git}, nil
}

// NewBareTestRepo creates a new bare git repository (for simulating remotes).
func NewBareTestRepo(name string) (*TestRepo, error) {
	dir, err := os.MkdirTemp("", "gastown-bare-"+name+"-*")
	if err != nil {
		return nil, err
	}

	git := NewRealGitOps(dir)

	// Initialize bare repo with main as default branch
	if _, err := git.run("init", "--bare", "-b", "main"); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	return &TestRepo{Path: dir, Git: git}, nil
}

// Cleanup removes the test repository.
func (r *TestRepo) Cleanup() {
	os.RemoveAll(r.Path)
}

// AddRemote adds a remote to this repo.
func (r *TestRepo) AddRemote(name, url string) error {
	_, err := r.Git.run("remote", "add", name, url)
	return err
}

// CreateInitialCommit creates an initial commit with a README.
func (r *TestRepo) CreateInitialCommit() (string, error) {
	return r.Git.AddCommit("Initial commit", map[string]string{
		"README.md": "# Test Repository\n",
	})
}

// CreateBranchWithCommit creates a branch and adds a commit.
func (r *TestRepo) CreateBranchWithCommit(branch, base, message string, files map[string]string) (string, error) {
	if err := r.Git.CreateBranch(branch, base); err != nil {
		return "", err
	}
	return r.Git.AddCommit(message, files)
}
