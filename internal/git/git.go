// Package git provides a wrapper for git operations via subprocess.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GitError contains raw output from a git command for agent observation.
// ZFC: Callers observe the raw output and decide what to do.
// The error interface methods provide human-readable messages, but agents
// should use Stdout/Stderr for programmatic observation.
type GitError struct {
	Command string // The git command that failed (e.g., "merge", "push")
	Args    []string
	Stdout  string // Raw stdout output
	Stderr  string // Raw stderr output
	Err     error  // Underlying error (e.g., exit code)
}

func (e *GitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("git %s: %s", e.Command, e.Stderr)
	}
	return fmt.Sprintf("git %s: %v", e.Command, e.Err)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

// moveDir moves a directory from src to dest. It first tries os.Rename for
// efficiency, but falls back to copy+delete if src and dest are on different
// filesystems (which causes EXDEV error on rename).
func moveDir(src, dest string) error {
	// Try rename first - works if same filesystem
	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	// Rename failed, use platform-specific copy for cross-filesystem moves
	if err := copyDirPreserving(src, dest); err != nil {
		return fmt.Errorf("copying directory: %w", err)
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("removing source after copy: %w", err)
	}
	return nil
}

// Git wraps git operations for a working directory.
type Git struct {
	workDir string
	gitDir  string // Optional: explicit git directory (for bare repos)
}

// NewGit creates a new Git wrapper for the given directory.
func NewGit(workDir string) *Git {
	return &Git{workDir: workDir}
}

// NewGitWithDir creates a Git wrapper with an explicit git directory.
// This is used for bare repos where gitDir points to the .git directory
// and workDir may be empty or point to a worktree.
func NewGitWithDir(gitDir, workDir string) *Git {
	return &Git{gitDir: gitDir, workDir: workDir}
}

// WorkDir returns the working directory for this Git instance.
func (g *Git) WorkDir() string {
	return g.workDir
}

// IsRepo returns true if the workDir is a git repository.
func (g *Git) IsRepo() bool {
	_, err := g.run("rev-parse", "--git-dir")
	return err == nil
}

// run executes a git command and returns stdout.
func (g *Git) run(args ...string) (string, error) {
	// If gitDir is set (bare repo), prepend --git-dir flag
	if g.gitDir != "" {
		args = append([]string{"--git-dir=" + g.gitDir}, args...)
	}

	cmd := exec.Command("git", args...)
	if g.workDir != "" {
		cmd.Dir = g.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", g.wrapError(err, stdout.String(), stderr.String(), args)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runWithEnv executes a git command with additional environment variables.
func (g *Git) runWithEnv(args []string, extraEnv []string) (string, error) {
	if g.gitDir != "" {
		args = append([]string{"--git-dir=" + g.gitDir}, args...)
	}
	cmd := exec.Command("git", args...)
	if g.workDir != "" {
		cmd.Dir = g.workDir
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", g.wrapError(err, stdout.String(), stderr.String(), args)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// wrapError wraps git errors with context.
// ZFC: Returns GitError with raw output for agent observation.
// Does not detect or interpret error types - agents should observe and decide.
func (g *Git) wrapError(err error, stdout, stderr string, args []string) error {
	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)

	// Determine command name (first arg, or first non-flag arg)
	command := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			command = arg
			break
		}
	}
	if command == "" && len(args) > 0 {
		command = args[0]
	}

	return &GitError{
		Command: command,
		Args:    args,
		Stdout:  stdout,
		Stderr:  stderr,
		Err:     err,
	}
}

// Clone clones a repository to the destination.
func (g *Git) Clone(url, dest string) error {
	// Ensure destination directory's parent exists
	destParent := filepath.Dir(dest)
	if err := os.MkdirAll(destParent, 0755); err != nil {
		return fmt.Errorf("creating destination parent: %w", err)
	}
	// Run clone from a temporary directory to completely isolate from any
	// git repo at the process cwd. Then move the result to the destination.
	tmpDir, err := os.MkdirTemp("", "gt-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpDest := filepath.Join(tmpDir, filepath.Base(dest))
	cmd := exec.Command("git", "clone", url, tmpDest)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+tmpDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stdout.String(), stderr.String(), []string{"clone", url})
	}

	// Move to final destination (handles cross-filesystem moves)
	if err := moveDir(tmpDest, dest); err != nil {
		return fmt.Errorf("moving clone to destination: %w", err)
	}

	// Configure hooks path for Gas Town clones
	if err := configureHooksPath(dest); err != nil {
		return err
	}
	// Initialize submodules if present
	return InitSubmodules(dest)
}

// CloneWithReference clones a repository using a local repo as an object reference.
// This saves disk by sharing objects without changing remotes.
func (g *Git) CloneWithReference(url, dest, reference string) error {
	// Ensure destination directory's parent exists
	destParent := filepath.Dir(dest)
	if err := os.MkdirAll(destParent, 0755); err != nil {
		return fmt.Errorf("creating destination parent: %w", err)
	}
	// Run clone from a temporary directory to completely isolate from any
	// git repo at the process cwd. Then move the result to the destination.
	tmpDir, err := os.MkdirTemp("", "gt-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpDest := filepath.Join(tmpDir, filepath.Base(dest))
	args := []string{"clone", "--reference-if-able", reference, url, tmpDest}
	if runtime.GOOS == "windows" {
		args = append([]string{"-c", "core.symlinks=true"}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+tmpDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stdout.String(), stderr.String(), args)
	}

	// Move to final destination (handles cross-filesystem moves)
	if err := moveDir(tmpDest, dest); err != nil {
		return fmt.Errorf("moving clone to destination: %w", err)
	}

	// Configure hooks path for Gas Town clones
	if err := configureHooksPath(dest); err != nil {
		return err
	}
	// Initialize submodules if present
	return InitSubmodules(dest)
}

// CloneBare clones a repository as a bare repo (no working directory).
// This is used for the shared repo architecture where all worktrees share a single git database.
func (g *Git) CloneBare(url, dest string) error {
	// Ensure destination directory's parent exists
	destParent := filepath.Dir(dest)
	if err := os.MkdirAll(destParent, 0755); err != nil {
		return fmt.Errorf("creating destination parent: %w", err)
	}
	// Run clone from a temporary directory to completely isolate from any
	// git repo at the process cwd. Then move the result to the destination.
	tmpDir, err := os.MkdirTemp("", "gt-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpDest := filepath.Join(tmpDir, filepath.Base(dest))
	cmd := exec.Command("git", "clone", "--bare", url, tmpDest)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+tmpDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stdout.String(), stderr.String(), []string{"clone", "--bare", url})
	}

	// Move to final destination (handles cross-filesystem moves)
	if err := moveDir(tmpDest, dest); err != nil {
		return fmt.Errorf("moving clone to destination: %w", err)
	}

	// Configure refspec so worktrees can fetch and see origin/* refs
	return configureRefspec(dest)
}

// configureHooksPath sets core.hooksPath to use the repo's .githooks directory
// if it exists. This ensures Gas Town agents use the pre-push hook that blocks
// pushes to non-main branches (internal PRs are not allowed).
func configureHooksPath(repoPath string) error {
	hooksDir := filepath.Join(repoPath, ".githooks")
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		// No .githooks directory, nothing to configure
		return nil
	}

	cmd := exec.Command("git", "-C", repoPath, "config", "core.hooksPath", ".githooks")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("configuring hooks path: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ConfigureHooksPath sets core.hooksPath for the repo/worktree if .githooks exists.
func (g *Git) ConfigureHooksPath() error {
	return configureHooksPath(g.workDir)
}

// configureRefspec sets remote.origin.fetch to the standard refspec for bare repos.
// Bare clones don't have this set by default, which breaks worktrees that need to
// fetch and see origin/* refs. Without this, `git fetch` only updates FETCH_HEAD
// and origin/main never appears in refs/remotes/origin/main.
// See: https://github.com/anthropics/gastown/issues/286
func configureRefspec(repoPath string) error {
	gitDir := repoPath
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		gitDir = filepath.Join(repoPath, ".git")
	}
	gitDir = filepath.Clean(gitDir)

	var stderr bytes.Buffer
	configCmd := exec.Command("git", "--git-dir", gitDir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	configCmd.Stderr = &stderr
	if err := configCmd.Run(); err != nil {
		return fmt.Errorf("configuring refspec: %s", strings.TrimSpace(stderr.String()))
	}

	fetchCmd := exec.Command("git", "--git-dir", gitDir, "fetch", "origin")
	fetchCmd.Stderr = &stderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("fetching origin: %s", strings.TrimSpace(stderr.String()))
	}

	return nil
}

// CloneBareWithReference clones a bare repository using a local repo as an object reference.
func (g *Git) CloneBareWithReference(url, dest, reference string) error {
	// Ensure destination directory's parent exists
	destParent := filepath.Dir(dest)
	if err := os.MkdirAll(destParent, 0755); err != nil {
		return fmt.Errorf("creating destination parent: %w", err)
	}
	// Run clone from a temporary directory to completely isolate from any
	// git repo at the process cwd. Then move the result to the destination.
	tmpDir, err := os.MkdirTemp("", "gt-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpDest := filepath.Join(tmpDir, filepath.Base(dest))
	cmd := exec.Command("git", "clone", "--bare", "--reference-if-able", reference, url, tmpDest)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+tmpDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stdout.String(), stderr.String(), []string{"clone", "--bare", "--reference-if-able", url})
	}

	// Move to final destination (handles cross-filesystem moves)
	if err := moveDir(tmpDest, dest); err != nil {
		return fmt.Errorf("moving clone to destination: %w", err)
	}

	// Configure refspec so worktrees can fetch and see origin/* refs
	return configureRefspec(dest)
}

// Checkout checks out the given ref.
func (g *Git) Checkout(ref string) error {
	_, err := g.run("checkout", ref)
	return err
}

// Fetch fetches from the remote.
func (g *Git) Fetch(remote string) error {
	_, err := g.run("fetch", remote)
	return err
}

// FetchPrune fetches from the remote and prunes stale remote-tracking refs.
// This removes remote-tracking branches for branches that no longer exist on the remote.
func (g *Git) FetchPrune(remote string) error {
	_, err := g.run("fetch", "--prune", remote)
	return err
}

// FetchBranch fetches a specific branch from the remote.
func (g *Git) FetchBranch(remote, branch string) error {
	_, err := g.run("fetch", remote, branch)
	return err
}

// Pull pulls from the remote branch.
func (g *Git) Pull(remote, branch string) error {
	_, err := g.run("pull", remote, branch)
	return err
}

// ConfigurePushURL sets the push URL for a remote while keeping the fetch URL.
// This is useful for read-only upstream repos where you want to push to a fork.
// Example: ConfigurePushURL("origin", "https://github.com/user/fork.git")
func (g *Git) ConfigurePushURL(remote, pushURL string) error {
	_, err := g.run("remote", "set-url", remote, "--push", pushURL)
	return err
}

// GetPushURL returns the push URL for a remote, or empty string if not configured.
func (g *Git) GetPushURL(remote string) (string, error) {
	out, err := g.run("remote", "get-url", "--push", remote)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Push pushes to the remote branch.
func (g *Git) Push(remote, branch string, force bool) error {
	args := []string{"push", remote, branch}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(args...)
	return err
}

// PushWithEnv pushes with additional environment variables.
// Used by gt mq integration land to set GT_INTEGRATION_LAND=1, which the
// pre-push hook checks to allow integration branch content landing on main.
func (g *Git) PushWithEnv(remote, branch string, force bool, env []string) error {
	args := []string{"push", remote, branch}
	if force {
		args = append(args, "--force")
	}
	_, err := g.runWithEnv(args, env)
	return err
}

// Add stages files for commit.
func (g *Git) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := g.run(args...)
	return err
}

// Commit creates a commit with the given message.
func (g *Git) Commit(message string) error {
	_, err := g.run("commit", "-m", message)
	return err
}

// CommitAll stages all changes and commits.
func (g *Git) CommitAll(message string) error {
	_, err := g.run("commit", "-am", message)
	return err
}

// GitStatus represents the status of the working directory.
type GitStatus struct {
	Clean    bool
	Modified []string
	Added    []string
	Deleted  []string
	Untracked []string
}

// Status returns the current git status.
func (g *Git) Status() (*GitStatus, error) {
	out, err := g.run("status", "--porcelain")
	if err != nil {
		return nil, err
	}

	status := &GitStatus{Clean: true}
	if out == "" {
		return status, nil
	}

	status.Clean = false
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		code := line[:2]
		file := line[3:]

		switch {
		case strings.Contains(code, "M"):
			status.Modified = append(status.Modified, file)
		case strings.Contains(code, "A"):
			status.Added = append(status.Added, file)
		case strings.Contains(code, "D"):
			status.Deleted = append(status.Deleted, file)
		case strings.Contains(code, "?"):
			status.Untracked = append(status.Untracked, file)
		}
	}

	return status, nil
}

// CurrentBranch returns the current branch name.
func (g *Git) CurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

// DefaultBranch returns the default branch name (what HEAD points to).
// This works for both regular and bare repositories.
// Returns "main" as fallback if detection fails.
func (g *Git) DefaultBranch() string {
	// Try symbolic-ref first (works for bare repos)
	branch, err := g.run("symbolic-ref", "--short", "HEAD")
	if err == nil && branch != "" {
		return branch
	}
	// Fallback to main
	return "main"
}

// RemoteDefaultBranch returns the default branch from the remote (origin).
// This is useful in worktrees where HEAD may not reflect the repo's actual default.
// Checks origin/HEAD first, then falls back to checking if master/main exists.
// Returns "main" as final fallback.
func (g *Git) RemoteDefaultBranch() string {
	// Try to get from origin/HEAD symbolic ref
	out, err := g.run("symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil && out != "" {
		// Returns refs/remotes/origin/main -> extract branch name
		parts := strings.Split(out, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if origin/master exists
	_, err = g.run("rev-parse", "--verify", "origin/master")
	if err == nil {
		return "master"
	}

	// Fallback: check if origin/main exists
	_, err = g.run("rev-parse", "--verify", "origin/main")
	if err == nil {
		return "main"
	}

	return "main" // final fallback
}

// HasUncommittedChanges returns true if there are uncommitted changes.
func (g *Git) HasUncommittedChanges() (bool, error) {
	status, err := g.Status()
	if err != nil {
		return false, err
	}
	return !status.Clean, nil
}

// RemoteURL returns the URL for the given remote.
func (g *Git) RemoteURL(remote string) (string, error) {
	return g.run("remote", "get-url", remote)
}

// AddRemote adds a new remote with the given name and URL.
func (g *Git) AddRemote(name, url string) (string, error) {
	return g.run("remote", "add", name, url)
}

// SetRemoteURL updates the URL for an existing remote.
func (g *Git) SetRemoteURL(name, url string) (string, error) {
	return g.run("remote", "set-url", name, url)
}

// Remotes returns the list of configured remote names.
func (g *Git) Remotes() ([]string, error) {
	out, err := g.run("remote")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ConfigGet returns the value of a git config key.
// Returns empty string if the key is not set.
func (g *Git) ConfigGet(key string) (string, error) {
	out, err := g.run("config", "--get", key)
	if err != nil {
		// git config --get returns exit code 1 if key not found
		return "", nil
	}
	return out, nil
}

// Merge merges the given branch into the current branch.
func (g *Git) Merge(branch string) error {
	_, err := g.run("merge", branch)
	return err
}

// MergeNoFF merges the given branch with --no-ff flag and a custom message.
func (g *Git) MergeNoFF(branch, message string) error {
	_, err := g.run("merge", "--no-ff", "-m", message, branch)
	return err
}

// MergeSquash performs a squash merge of the given branch and commits with the provided message.
// This stages all changes from the branch without creating a merge commit, then commits them
// as a single commit with the given message. This eliminates redundant merge commits while
// preserving the original commit message from the source branch.
func (g *Git) MergeSquash(branch, message string) error {
	// Stage all changes from the branch without committing
	if _, err := g.run("merge", "--squash", branch); err != nil {
		return err
	}
	// Commit the staged changes with the provided message
	_, err := g.run("commit", "-m", message)
	return err
}

// GetBranchCommitMessage returns the commit message of the HEAD commit on the given branch.
// This is useful for preserving the original conventional commit message (feat:/fix:) when
// performing squash merges.
func (g *Git) GetBranchCommitMessage(branch string) (string, error) {
	return g.run("log", "-1", "--format=%B", branch)
}

// DeleteRemoteBranch deletes a branch on the remote.
func (g *Git) DeleteRemoteBranch(remote, branch string) error {
	_, err := g.run("push", remote, "--delete", branch)
	return err
}

// Rebase rebases the current branch onto the given ref.
func (g *Git) Rebase(onto string) error {
	_, err := g.run("rebase", onto)
	return err
}

// AbortMerge aborts a merge in progress.
func (g *Git) AbortMerge() error {
	_, err := g.run("merge", "--abort")
	return err
}

// CheckConflicts performs a test merge to check if source can be merged into target
// without conflicts. Returns a list of conflicting files, or empty slice if clean.
// The merge is always aborted after checking - no actual changes are made.
//
// The caller must ensure the working directory is clean before calling this.
// After return, the working directory is restored to the target branch.
func (g *Git) CheckConflicts(source, target string) ([]string, error) {
	// Checkout the target branch
	if err := g.Checkout(target); err != nil {
		return nil, fmt.Errorf("checkout target %s: %w", target, err)
	}

	// Attempt test merge with --no-commit --no-ff
	// We need to capture both stdout and stderr to detect conflicts
	_, mergeErr := g.runMergeCheck("merge", "--no-commit", "--no-ff", source)

	if mergeErr != nil {
		// ZFC: Use git's porcelain output to detect conflicts instead of parsing stderr.
		// GetConflictingFiles() uses `git diff --diff-filter=U` which is the proper way.
		conflicts, err := g.GetConflictingFiles()
		if err == nil && len(conflicts) > 0 {
			// Abort the test merge (best-effort cleanup)
			_ = g.AbortMerge()
			return conflicts, nil
		}

		// No unmerged files detected - this is some other merge error
		_ = g.AbortMerge()
		return nil, mergeErr
	}

	// Merge succeeded (no conflicts) - abort the test merge
	// Use reset since --abort won't work on successful merge (best-effort cleanup)
	_, _ = g.run("reset", "--hard", "HEAD")
	return nil, nil
}

// runMergeCheck runs a git merge command and returns error info from both stdout and stderr.
// ZFC: Returns GitError with raw output for agent observation.
func (g *Git) runMergeCheck(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// ZFC: Return raw output for observation, don't interpret CONFLICT
		return "", g.wrapError(err, stdout.String(), stderr.String(), args)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetConflictingFiles returns the list of files with merge conflicts.
// ZFC: Uses git's porcelain output (diff --diff-filter=U) instead of parsing stderr.
// This is the proper way to detect conflicts without violating ZFC.
func (g *Git) GetConflictingFiles() ([]string, error) {
	// git diff --name-only --diff-filter=U shows unmerged files
	out, err := g.run("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	files := strings.Split(out, "\n")
	// Filter out empty strings
	var result []string
	for _, f := range files {
		if f != "" {
			result = append(result, f)
		}
	}
	return result, nil
}

// AbortRebase aborts a rebase in progress.
func (g *Git) AbortRebase() error {
	_, err := g.run("rebase", "--abort")
	return err
}

// CreateBranch creates a new branch.
func (g *Git) CreateBranch(name string) error {
	_, err := g.run("branch", name)
	return err
}

// CreateBranchFrom creates a new branch from a specific ref.
func (g *Git) CreateBranchFrom(name, ref string) error {
	_, err := g.run("branch", name, ref)
	return err
}

// BranchExists checks if a branch exists locally.
func (g *Git) BranchExists(name string) (bool, error) {
	_, err := g.run("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err != nil {
		// Exit code 1 means branch doesn't exist
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RefExists checks if a ref exists (works for any ref including origin/<branch>).
// Uses show-ref for fully-qualified refs, falls back to rev-parse for short refs.
func (g *Git) RefExists(ref string) (bool, error) {
	// Fully-qualified refs (refs/...) use show-ref which has a stable exit code contract:
	// exit 0 = exists, exit 1 = missing, exit >1 = error.
	if strings.HasPrefix(ref, "refs/") {
		_, err := g.run("show-ref", "--verify", "--quiet", ref)
		if err != nil {
			if strings.Contains(err.Error(), "exit status 1") {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	// Short refs (e.g., origin/main) need rev-parse --verify.
	_, err := g.run("rev-parse", "--verify", ref)
	if err != nil {
		// Only treat "ref missing" as false — propagate other failures
		// (e.g. corrupted repo, permissions, disk I/O).
		var gitErr *GitError
		if errors.As(err, &gitErr) &&
			strings.Contains(gitErr.Stderr, "Needed a single revision") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsEmpty returns true if the repository has no refs (an empty/unborn repo).
// This is the case for newly-created repos with no commits.
func (g *Git) IsEmpty() (bool, error) {
	out, err := g.run("show-ref")
	if err != nil {
		// git show-ref exits 1 when there are no refs — that means empty
		if strings.Contains(err.Error(), "exit status 1") {
			return true, nil
		}
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// RemoteBranchExists checks if a branch exists on the remote.
func (g *Git) RemoteBranchExists(remote, branch string) (bool, error) {
	out, err := g.run("ls-remote", "--heads", remote, branch)
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// RemoteTrackingBranchExists checks if a remote-tracking branch ref exists locally
// (e.g. refs/remotes/origin/main), without hitting the network.
func (g *Git) RemoteTrackingBranchExists(remote, branch string) (bool, error) {
	ref := fmt.Sprintf("refs/remotes/%s/%s", remote, branch)
	_, err := g.run("show-ref", "--verify", "--quiet", ref)
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := g.run("branch", flag, name)
	return err
}

// ListBranches returns all local branches matching a pattern.
// Pattern uses git's pattern matching (e.g., "polecat/*" matches all polecat branches).
// Returns branch names without the refs/heads/ prefix.
func (g *Git) ListBranches(pattern string) ([]string, error) {
	args := []string{"branch", "--list", "--format=%(refname:short)"}
	if pattern != "" {
		args = append(args, pattern)
	}
	out, err := g.run(args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ResetBranch force-updates a branch to point to a ref.
// This is useful for resetting stale polecat branches to main.
// NOTE: This uses `git branch -f` which fails on the currently checked-out branch.
// Use ResetHard instead when the target branch is checked out.
func (g *Git) ResetBranch(name, ref string) error {
	_, err := g.run("branch", "-f", name, ref)
	return err
}

// ResetHard resets the current working tree and index to the given ref.
// Unlike ResetBranch, this works on the currently checked-out branch.
func (g *Git) ResetHard(ref string) error {
	_, err := g.run("reset", "--hard", ref)
	return err
}

// Rev returns the commit hash for the given ref.
func (g *Git) Rev(ref string) (string, error) {
	return g.run("rev-parse", ref)
}

// IsAncestor checks if ancestor is an ancestor of descendant.
func (g *Git) IsAncestor(ancestor, descendant string) (bool, error) {
	_, err := g.run("merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil {
		// Exit code 1 means not an ancestor, not an error
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// WorktreeAdd creates a new worktree at the given path with a new branch.
// The new branch is created from the current HEAD.
func (g *Git) WorktreeAdd(path, branch string) error {
	if _, err := g.run("worktree", "add", "-b", branch, path); err != nil {
		return err
	}
	return InitSubmodules(path)
}

// WorktreeAddFromRef creates a new worktree at the given path with a new branch
// starting from the specified ref (e.g., "origin/main").
func (g *Git) WorktreeAddFromRef(path, branch, startPoint string) error {
	if _, err := g.run("worktree", "add", "-b", branch, path, startPoint); err != nil {
		return err
	}
	return InitSubmodules(path)
}

// WorktreeAddDetached creates a new worktree at the given path with a detached HEAD.
func (g *Git) WorktreeAddDetached(path, ref string) error {
	if _, err := g.run("worktree", "add", "--detach", path, ref); err != nil {
		return err
	}
	return InitSubmodules(path)
}

// WorktreeAddExisting creates a new worktree at the given path for an existing branch.
func (g *Git) WorktreeAddExisting(path, branch string) error {
	if _, err := g.run("worktree", "add", path, branch); err != nil {
		return err
	}
	return InitSubmodules(path)
}

// WorktreeAddExistingForce creates a new worktree even if the branch is already checked out elsewhere.
// This is useful for cross-rig worktrees where multiple clones need to be on main.
func (g *Git) WorktreeAddExistingForce(path, branch string) error {
	if _, err := g.run("worktree", "add", "--force", path, branch); err != nil {
		return err
	}
	return InitSubmodules(path)
}

// IsSparseCheckoutConfigured checks if sparse checkout is enabled for a given repo/worktree.
// This is used by doctor to detect legacy sparse checkout configurations that should be removed.
func IsSparseCheckoutConfigured(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "config", "core.sparseCheckout")
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

// RemoveSparseCheckout disables sparse checkout for a repo/worktree and restores all files.
// This is used by doctor to clean up legacy sparse checkout configurations.
func RemoveSparseCheckout(repoPath string) error {
	// Use git sparse-checkout disable which properly restores hidden files
	cmd := exec.Command("git", "-C", repoPath, "sparse-checkout", "disable")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("disabling sparse checkout: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

// WorktreeRemove removes a worktree.
func (g *Git) WorktreeRemove(path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(args...)
	return err
}

// WorktreePrune removes worktree entries for deleted paths.
func (g *Git) WorktreePrune() error {
	_, err := g.run("worktree", "prune")
	return err
}

// Worktree represents a git worktree.
type Worktree struct {
	Path   string
	Branch string
	Commit string
}

// WorktreeList returns all worktrees for this repository.
func (g *Git) WorktreeList() ([]Worktree, error) {
	out, err := g.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}

	// Don't forget the last one
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// BranchCreatedDate returns the date when a branch was created.
// This uses the committer date of the first commit on the branch.
// Returns date in YYYY-MM-DD format.
func (g *Git) BranchCreatedDate(branch string) (string, error) {
	// Get the date of the first commit on the branch that's not on the default branch
	// Use merge-base to find where the branch diverged
	defaultBranch := g.RemoteDefaultBranch()
	mergeBase, err := g.run("merge-base", defaultBranch, branch)
	if err != nil {
		// If merge-base fails, fall back to the branch tip's date
		out, err := g.run("log", "-1", "--format=%cs", branch)
		if err != nil {
			return "", err
		}
		return out, nil
	}

	// Get the first commit after the merge base on this branch
	out, err := g.run("log", "--format=%cs", "--reverse", mergeBase+".."+branch)
	if err != nil {
		return "", err
	}

	// Get the first line (first commit's date)
	lines := strings.Split(out, "\n")
	if len(lines) > 0 && lines[0] != "" {
		return lines[0], nil
	}

	// If no commits after merge-base, the branch points to merge-base
	// Return the merge-base commit date
	out, err = g.run("log", "-1", "--format=%cs", mergeBase)
	if err != nil {
		return "", err
	}
	return out, nil
}

// CommitsAhead returns the number of commits that branch has ahead of base.
// For example, CommitsAhead("main", "feature") returns how many commits
// are on feature that are not on main.
func (g *Git) CommitsAhead(base, branch string) (int, error) {
	out, err := g.run("rev-list", "--count", base+".."+branch)
	if err != nil {
		return 0, err
	}

	var count int
	_, err = fmt.Sscanf(out, "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("parsing commit count: %w", err)
	}

	return count, nil
}

// CountCommitsBehind returns the number of commits that HEAD is behind the given ref.
// For example, CountCommitsBehind("origin/main") returns how many commits
// are on origin/main that are not on the current HEAD.
func (g *Git) CountCommitsBehind(ref string) (int, error) {
	out, err := g.run("rev-list", "--count", "HEAD.."+ref)
	if err != nil {
		return 0, err
	}

	var count int
	_, err = fmt.Sscanf(out, "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("parsing commit count: %w", err)
	}

	return count, nil
}

// StashCount returns the number of stashes in the repository.
func (g *Git) StashCount() (int, error) {
	out, err := g.run("stash", "list")
	if err != nil {
		return 0, err
	}

	if out == "" {
		return 0, nil
	}

	// Count lines in the stash list
	lines := strings.Split(out, "\n")
	count := 0
	for _, line := range lines {
		if line != "" {
			count++
		}
	}
	return count, nil
}

// UnpushedCommits returns the number of commits that are not pushed to the remote.
// It checks if the current branch has an upstream and counts commits ahead.
// Returns 0 if there is no upstream configured.
func (g *Git) UnpushedCommits() (int, error) {
	// Get the upstream branch
	upstream, err := g.run("rev-parse", "--abbrev-ref", "@{u}")
	if err != nil {
		// No upstream configured - this is common for polecat branches
		// Check if we can compare against origin/main instead
		// If we can't get any reference, return 0 (benefit of the doubt)
		return 0, nil
	}

	// Count commits between upstream and HEAD
	out, err := g.run("rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		return 0, err
	}

	var count int
	_, err = fmt.Sscanf(out, "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("parsing unpushed count: %w", err)
	}

	return count, nil
}

// UncommittedWorkStatus contains information about uncommitted work in a repo.
type UncommittedWorkStatus struct {
	HasUncommittedChanges bool
	StashCount            int
	UnpushedCommits       int
	// Details for error messages
	ModifiedFiles   []string
	UntrackedFiles  []string
}

// Clean returns true if there is no uncommitted work.
func (s *UncommittedWorkStatus) Clean() bool {
	return !s.HasUncommittedChanges && s.StashCount == 0 && s.UnpushedCommits == 0
}

// CleanExcludingBeads returns true if the only uncommitted changes are .beads/ files.
// This is useful for polecat stale detection where beads database files are synced
// across worktrees and shouldn't block cleanup.
func (s *UncommittedWorkStatus) CleanExcludingBeads() bool {
	// Stashes and unpushed commits always count as uncommitted work
	if s.StashCount > 0 || s.UnpushedCommits > 0 {
		return false
	}

	// Check if all modified files are beads files
	for _, f := range s.ModifiedFiles {
		if !isBeadsPath(f) {
			return false
		}
	}

	// Check if all untracked files are beads files
	for _, f := range s.UntrackedFiles {
		if !isBeadsPath(f) {
			return false
		}
	}

	return true
}

// isBeadsPath returns true if the path is a .beads/ file.
func isBeadsPath(path string) bool {
	return strings.Contains(path, ".beads/") || strings.Contains(path, ".beads\\")
}

// String returns a human-readable summary of uncommitted work.
func (s *UncommittedWorkStatus) String() string {
	var issues []string
	if s.HasUncommittedChanges {
		issues = append(issues, fmt.Sprintf("%d uncommitted change(s)", len(s.ModifiedFiles)+len(s.UntrackedFiles)))
	}
	if s.StashCount > 0 {
		issues = append(issues, fmt.Sprintf("%d stash(es)", s.StashCount))
	}
	if s.UnpushedCommits > 0 {
		issues = append(issues, fmt.Sprintf("%d unpushed commit(s)", s.UnpushedCommits))
	}
	if len(issues) == 0 {
		return "clean"
	}
	return strings.Join(issues, ", ")
}

// CheckUncommittedWork performs a comprehensive check for uncommitted work.
func (g *Git) CheckUncommittedWork() (*UncommittedWorkStatus, error) {
	status := &UncommittedWorkStatus{}

	// Check git status
	gitStatus, err := g.Status()
	if err != nil {
		return nil, fmt.Errorf("checking git status: %w", err)
	}
	status.HasUncommittedChanges = !gitStatus.Clean
	status.ModifiedFiles = append(gitStatus.Modified, gitStatus.Added...)
	status.ModifiedFiles = append(status.ModifiedFiles, gitStatus.Deleted...)
	status.UntrackedFiles = gitStatus.Untracked

	// Check stashes
	stashCount, err := g.StashCount()
	if err != nil {
		return nil, fmt.Errorf("checking stashes: %w", err)
	}
	status.StashCount = stashCount

	// Check unpushed commits
	unpushed, err := g.UnpushedCommits()
	if err != nil {
		return nil, fmt.Errorf("checking unpushed commits: %w", err)
	}
	status.UnpushedCommits = unpushed

	return status, nil
}

// BranchPushedToRemote checks if a branch has been pushed to the remote.
// Returns (pushed bool, unpushedCount int, err).
// This handles polecat branches that don't have upstream tracking configured.
func (g *Git) BranchPushedToRemote(localBranch, remote string) (bool, int, error) {
	remoteBranch := remote + "/" + localBranch

	// Check if the remote branch exists via ls-remote and save the output.
	// The output contains the SHA which we reuse in the fallback path below,
	// avoiding a redundant second ls-remote call.
	lsOut, err := g.run("ls-remote", "--heads", remote, localBranch)
	if err != nil {
		return false, 0, fmt.Errorf("checking remote branch: %w", err)
	}

	if lsOut == "" {
		// Remote branch doesn't exist - count commits since origin/main (or HEAD if that fails)
		count, err := g.run("rev-list", "--count", "origin/main..HEAD")
		if err != nil {
			// Fallback: just count all commits on HEAD
			count, err = g.run("rev-list", "--count", "HEAD")
			if err != nil {
				return false, 0, fmt.Errorf("counting commits: %w", err)
			}
		}
		var n int
		_, err = fmt.Sscanf(count, "%d", &n)
		if err != nil {
			return false, 0, fmt.Errorf("parsing commit count: %w", err)
		}
		// If there are any commits since main, branch is not pushed
		return n == 0, n, nil
	}

	// Remote branch exists - fetch to ensure we have the local tracking ref
	// This handles the case where we just pushed and origin/branch doesn't exist locally yet
	_, fetchErr := g.run("fetch", remote, localBranch)

	// In worktrees, the fetch may not update refs/remotes/origin/<branch> due to
	// missing refspecs. If the remote ref doesn't exist locally, create it from FETCH_HEAD.
	// See: gt-cehl8 (gt done fails in worktrees due to missing origin tracking ref)
	remoteRef := "refs/remotes/" + remoteBranch
	if _, err := g.run("rev-parse", "--verify", remoteRef); err != nil {
		// Remote ref doesn't exist locally - update it from FETCH_HEAD if fetch succeeded.
		// Best-effort: if this fails, the code below falls back to the saved ls-remote SHA.
		if fetchErr == nil {
			_, _ = g.run("update-ref", remoteRef, "FETCH_HEAD")
		}
	}

	// Check if local is ahead
	count, err := g.run("rev-list", "--count", remoteBranch+"..HEAD")
	if err != nil {
		// Fallback: If we can't use the tracking ref (possibly missing remote.origin.fetch),
		// use the SHA from the ls-remote call above instead of hitting the network again.
		// See: gt-0eh3r (gt done fails in worktree with missing remote.origin.fetch config)
		parts := strings.Fields(strings.TrimSpace(lsOut))
		if len(parts) == 0 {
			return false, 0, fmt.Errorf("counting unpushed commits: %w (invalid ls-remote output)", err)
		}
		remoteSHA := parts[0]

		// Count commits from remote SHA to HEAD
		count, err = g.run("rev-list", "--count", remoteSHA+"..HEAD")
		if err != nil {
			return false, 0, fmt.Errorf("counting unpushed commits (fallback): %w", err)
		}
	}

	var n int
	_, err = fmt.Sscanf(count, "%d", &n)
	if err != nil {
		return false, 0, fmt.Errorf("parsing unpushed count: %w", err)
	}

	return n == 0, n, nil
}

// PrunedBranch represents a local branch that was pruned (or would be pruned in dry-run).
type PrunedBranch struct {
	Name   string // Branch name (e.g., "polecat/rictus-mkb0vq9f")
	Reason string // Why it was pruned: "merged", "no-remote", "no-remote-merged"
}

// PruneStaleBranches finds and deletes local branches matching a pattern that are
// stale — either fully merged to the default branch or whose remote tracking branch
// no longer exists (indicating the remote branch was deleted after merge).
//
// This addresses cross-clone branch accumulation: when polecats push branches to
// origin, other clones create local tracking branches via git fetch. After the
// remote branch is deleted (post-merge), git fetch --prune removes the remote
// tracking ref but the local branch persists indefinitely.
//
// Safety: never deletes the current branch or the default branch (main/master).
// Uses git branch -d (not -D), so only fully-merged branches are deleted.
func (g *Git) PruneStaleBranches(pattern string, dryRun bool) ([]PrunedBranch, error) {
	if pattern == "" {
		pattern = "polecat/*"
	}

	// Get current branch to avoid deleting it
	currentBranch, _ := g.CurrentBranch()
	defaultBranch := g.RemoteDefaultBranch()

	// List all local branches matching the pattern
	branches, err := g.ListBranches(pattern)
	if err != nil {
		return nil, fmt.Errorf("listing branches: %w", err)
	}

	var pruned []PrunedBranch
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		if branch == "" || branch == currentBranch || branch == defaultBranch {
			continue
		}

		// Check if the remote tracking branch still exists
		hasRemote, err := g.RemoteTrackingBranchExists("origin", branch)
		if err != nil {
			continue // Skip on error, don't fail the whole operation
		}

		// Check if the branch is merged to the default branch
		merged, err := g.IsAncestor(branch, "origin/"+defaultBranch)
		if err != nil {
			// If we can't determine merge status, only prune if remote is gone
			if hasRemote {
				continue
			}
			// Remote gone and can't check merge status — skip to be safe
			continue
		}

		var reason string
		if merged && !hasRemote {
			reason = "no-remote-merged"
		} else if merged {
			reason = "merged"
		} else if !hasRemote {
			reason = "no-remote"
		} else {
			continue // Branch has remote and is not merged — keep it
		}

		if !dryRun {
			// Use -d (not -D) for safety — only deletes fully merged branches.
			// For "no-remote" branches that aren't merged, -d will fail safely.
			if err := g.DeleteBranch(branch, false); err != nil {
				// If -d fails (not merged), skip this branch
				continue
			}
		}

		pruned = append(pruned, PrunedBranch{
			Name:   branch,
			Reason: reason,
		})
	}

	return pruned, nil
}

// SubmoduleChange represents a changed submodule pointer between two refs.
type SubmoduleChange struct {
	Path   string // Submodule path relative to repo root
	OldSHA string // Previous commit SHA (or empty for new submodule)
	NewSHA string // New commit SHA (or empty for removed submodule)
	URL    string // Submodule remote URL from .gitmodules
}

// InitSubmodules initializes and updates submodules if .gitmodules exists.
// This is a no-op for repos without submodules.
func InitSubmodules(repoPath string) error {
	gitmodules := filepath.Join(repoPath, ".gitmodules")
	if _, err := os.Stat(gitmodules); os.IsNotExist(err) {
		return nil
	}
	cmd := exec.Command("git", "-C", repoPath, "submodule", "update", "--init", "--recursive")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("initializing submodules: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

// SubmoduleChanges detects submodule pointer changes between two refs.
// Returns nil if no submodules changed or if the repo has no submodules.
func (g *Git) SubmoduleChanges(base, head string) ([]SubmoduleChange, error) {
	// git diff --raw shows mode 160000 for gitlink (submodule) entries
	out, err := g.run("diff", "--raw", base, head)
	if err != nil {
		return nil, fmt.Errorf("diffing for submodule changes: %w", err)
	}
	if out == "" {
		return nil, nil
	}

	var changes []SubmoduleChange
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: :oldmode newmode oldsha newsha status\tpath
		// Submodule entries have mode 160000
		if !strings.Contains(line, "160000") {
			continue
		}
		// Parse the raw diff line
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		path := strings.TrimSpace(parts[1])
		fields := strings.Fields(parts[0])
		if len(fields) < 5 {
			continue
		}
		oldSHA := fields[2]
		newSHA := fields[3]
		// Null SHAs (all zeros) indicate added/removed submodules
		if strings.Repeat("0", len(oldSHA)) == oldSHA {
			oldSHA = ""
		}
		if strings.Repeat("0", len(newSHA)) == newSHA {
			newSHA = ""
		}

		change := SubmoduleChange{
			Path:   path,
			OldSHA: oldSHA,
			NewSHA: newSHA,
		}

		// Try to get the submodule URL from .gitmodules on the head ref
		url, urlErr := g.submoduleURL(head, path)
		if urlErr == nil {
			change.URL = url
		}

		changes = append(changes, change)
	}
	return changes, nil
}

// submoduleURL reads the URL for a submodule from .gitmodules at a given ref.
// Uses git config -f to parse the file correctly regardless of field ordering.
func (g *Git) submoduleURL(ref, submodulePath string) (string, error) {
	// Write .gitmodules from the ref to a temp file so we can use git config -f
	content, err := g.run("show", ref+":.gitmodules")
	if err != nil {
		return "", err
	}
	tmpFile, err := os.CreateTemp("", "gitmodules-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file for .gitmodules: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("writing temp .gitmodules: %w", err)
	}
	tmpFile.Close()

	// List all submodule.<name>.path entries to find the section matching our path
	cmd := exec.Command("git", "config", "-f", tmpFile.Name(), "--get-regexp", `^submodule\..*\.path$`)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("reading submodule paths from .gitmodules: %w", err)
	}

	var sectionName string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: submodule.<name>.path <value>
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == submodulePath {
			key := parts[0]
			key = strings.TrimPrefix(key, "submodule.")
			key = strings.TrimSuffix(key, ".path")
			sectionName = key
			break
		}
	}
	if sectionName == "" {
		return "", fmt.Errorf("submodule URL not found for path %s", submodulePath)
	}

	// Get the URL for this section
	urlCmd := exec.Command("git", "config", "-f", tmpFile.Name(), "--get", "submodule."+sectionName+".url")
	var urlOut bytes.Buffer
	urlCmd.Stdout = &urlOut
	if err := urlCmd.Run(); err != nil {
		return "", fmt.Errorf("reading URL for submodule %s: %w", sectionName, err)
	}
	url := strings.TrimSpace(urlOut.String())
	if url == "" {
		return "", fmt.Errorf("submodule URL not found for path %s", submodulePath)
	}
	return url, nil
}

// PushSubmoduleCommit pushes a specific commit SHA from a submodule to its remote.
// The submodulePath is relative to the repo working directory.
// The commit must exist in the submodule's object store (shared via .repo.git/modules/).
func (g *Git) PushSubmoduleCommit(submodulePath, sha, remote string) error {
	absPath := filepath.Join(g.workDir, submodulePath)
	// Detect the remote's default branch (don't assume main)
	defaultBranch, err := submoduleDefaultBranch(absPath, remote)
	if err != nil {
		return fmt.Errorf("detecting default branch for submodule %s: %w", submodulePath, err)
	}
	cmd := exec.Command("git", "-C", absPath, "push", remote, sha+":refs/heads/"+defaultBranch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pushing submodule %s commit %s: %s", submodulePath, sha[:8], strings.TrimSpace(stderr.String()))
	}
	return nil
}

// submoduleDefaultBranch detects the default branch of a submodule's remote.
// Tries local refs first to avoid network round-trips, falling back to remote queries.
func submoduleDefaultBranch(submodulePath, remote string) (string, error) {
	// Try local symbolic-ref first (no network, fastest)
	symCmd := exec.Command("git", "-C", submodulePath, "symbolic-ref", "refs/remotes/"+remote+"/HEAD")
	if symOut, err := symCmd.Output(); err == nil {
		ref := strings.TrimSpace(string(symOut))
		// refs/remotes/origin/HEAD -> refs/remotes/origin/main -> main
		if parts := strings.Split(ref, "/"); len(parts) > 0 {
			branch := parts[len(parts)-1]
			if branch != "" {
				return branch, nil
			}
		}
	}

	// Try local tracking refs (no network)
	for _, candidate := range []string{"main", "master"} {
		check := exec.Command("git", "-C", submodulePath, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+candidate)
		if check.Run() == nil {
			return candidate, nil
		}
	}

	// Fallback: network query via ls-remote
	for _, candidate := range []string{"main", "master"} {
		check := exec.Command("git", "-C", submodulePath, "ls-remote", "--exit-code", remote, "refs/heads/"+candidate)
		if check.Run() == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch for remote %s", remote)
}
