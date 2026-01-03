// Package git provides a wrapper for git operations via subprocess.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Common errors
var (
	ErrNotARepo       = errors.New("not a git repository")
	ErrMergeConflict  = errors.New("merge conflict")
	ErrAuthFailure    = errors.New("authentication failed")
	ErrRebaseConflict = errors.New("rebase conflict")
)

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
		return "", g.wrapError(err, stderr.String(), args)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// wrapError wraps git errors with context.
func (g *Git) wrapError(err error, stderr string, args []string) error {
	stderr = strings.TrimSpace(stderr)

	// Detect specific error types
	if strings.Contains(stderr, "not a git repository") {
		return ErrNotARepo
	}
	if strings.Contains(stderr, "CONFLICT") || strings.Contains(stderr, "Merge conflict") {
		return ErrMergeConflict
	}
	if strings.Contains(stderr, "Authentication failed") || strings.Contains(stderr, "could not read Username") {
		return ErrAuthFailure
	}
	if strings.Contains(stderr, "needs merge") || strings.Contains(stderr, "rebase in progress") {
		return ErrRebaseConflict
	}

	if stderr != "" {
		return fmt.Errorf("git %s: %s", args[0], stderr)
	}
	return fmt.Errorf("git %s: %w", args[0], err)
}

// Clone clones a repository to the destination.
func (g *Git) Clone(url, dest string) error {
	cmd := exec.Command("git", "clone", url, dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stderr.String(), []string{"clone", url})
	}
	return nil
}

// CloneWithReference clones a repository using a local repo as an object reference.
// This saves disk by sharing objects without changing remotes.
func (g *Git) CloneWithReference(url, dest, reference string) error {
	cmd := exec.Command("git", "clone", "--reference-if-able", reference, url, dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stderr.String(), []string{"clone", "--reference-if-able", url})
	}
	return nil
}

// CloneBare clones a repository as a bare repo (no working directory).
// This is used for the shared repo architecture where all worktrees share a single git database.
func (g *Git) CloneBare(url, dest string) error {
	cmd := exec.Command("git", "clone", "--bare", url, dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stderr.String(), []string{"clone", "--bare", url})
	}
	return nil
}

// CloneBareWithReference clones a bare repository using a local repo as an object reference.
func (g *Git) CloneBareWithReference(url, dest, reference string) error {
	cmd := exec.Command("git", "clone", "--bare", "--reference-if-able", reference, url, dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return g.wrapError(err, stderr.String(), []string{"clone", "--bare", "--reference-if-able", url})
	}
	return nil
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

// Push pushes to the remote branch.
func (g *Git) Push(remote, branch string, force bool) error {
	args := []string{"push", remote, branch}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(args...)
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
		// Check if there are unmerged files (indicates conflict)
		conflicts, err := g.getConflictingFiles()
		if err == nil && len(conflicts) > 0 {
			// Abort the test merge (best-effort cleanup)
			_ = g.AbortMerge()
			return conflicts, nil
		}

		// Check if it's a conflict error from wrapper
		if errors.Is(mergeErr, ErrMergeConflict) {
			_ = g.AbortMerge() // best-effort cleanup
			return conflicts, nil
		}

		// Some other merge error (best-effort cleanup)
		_ = g.AbortMerge()
		return nil, mergeErr
	}

	// Merge succeeded (no conflicts) - abort the test merge
	// Use reset since --abort won't work on successful merge (best-effort cleanup)
	_, _ = g.run("reset", "--hard", "HEAD")
	return nil, nil
}

// runMergeCheck runs a git merge command and returns error info from both stdout and stderr.
// This is needed because git merge outputs CONFLICT info to stdout.
func (g *Git) runMergeCheck(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check stdout for CONFLICT message (git sends it there)
		stdoutStr := stdout.String()
		if strings.Contains(stdoutStr, "CONFLICT") {
			return "", ErrMergeConflict
		}
		// Fall back to stderr check
		return "", g.wrapError(err, stderr.String(), args)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// getConflictingFiles returns the list of files with merge conflicts.
func (g *Git) getConflictingFiles() ([]string, error) {
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

// RemoteBranchExists checks if a branch exists on the remote.
func (g *Git) RemoteBranchExists(remote, branch string) (bool, error) {
	_, err := g.run("ls-remote", "--heads", remote, branch)
	if err != nil {
		return false, err
	}
	// ls-remote returns empty if branch doesn't exist, need to check output
	out, err := g.run("ls-remote", "--heads", remote, branch)
	if err != nil {
		return false, err
	}
	return out != "", nil
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
func (g *Git) ResetBranch(name, ref string) error {
	_, err := g.run("branch", "-f", name, ref)
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
	_, err := g.run("worktree", "add", "-b", branch, path)
	return err
}

// WorktreeAddDetached creates a new worktree at the given path with a detached HEAD.
func (g *Git) WorktreeAddDetached(path, ref string) error {
	_, err := g.run("worktree", "add", "--detach", path, ref)
	return err
}

// WorktreeAddExisting creates a new worktree at the given path for an existing branch.
func (g *Git) WorktreeAddExisting(path, branch string) error {
	_, err := g.run("worktree", "add", path, branch)
	return err
}

// WorktreeAddExistingForce creates a new worktree even if the branch is already checked out elsewhere.
// This is useful for cross-rig worktrees where multiple clones need to be on main.
func (g *Git) WorktreeAddExistingForce(path, branch string) error {
	_, err := g.run("worktree", "add", "--force", path, branch)
	return err
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
	// Get the date of the first commit on the branch that's not on main
	// Use merge-base to find where the branch diverged from main
	mergeBase, err := g.run("merge-base", "main", branch)
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

	// First check if the remote branch exists
	exists, err := g.RemoteBranchExists(remote, localBranch)
	if err != nil {
		return false, 0, fmt.Errorf("checking remote branch: %w", err)
	}

	if !exists {
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
		// Remote ref doesn't exist locally - update it from FETCH_HEAD if fetch succeeded
		if fetchErr == nil {
			_, _ = g.run("update-ref", remoteRef, "FETCH_HEAD")
		}
	}

	// Check if local is ahead
	count, err := g.run("rev-list", "--count", remoteBranch+"..HEAD")
	if err != nil {
		// Fallback: If we can't use the tracking ref (possibly missing remote.origin.fetch),
		// get the remote commit SHA directly via ls-remote and compare.
		// See: gt-0eh3r (gt done fails in worktree with missing remote.origin.fetch config)
		remoteSHA, lsErr := g.run("ls-remote", remote, "refs/heads/"+localBranch)
		if lsErr != nil {
			return false, 0, fmt.Errorf("counting unpushed commits: %w (fallback also failed: %v)", err, lsErr)
		}
		// Parse SHA from ls-remote output (format: "<sha>\trefs/heads/<branch>")
		remoteSHA = strings.TrimSpace(remoteSHA)
		if remoteSHA == "" {
			return false, 0, fmt.Errorf("counting unpushed commits: %w (remote branch not found)", err)
		}
		parts := strings.Fields(remoteSHA)
		if len(parts) == 0 {
			return false, 0, fmt.Errorf("counting unpushed commits: %w (invalid ls-remote output)", err)
		}
		remoteSHA = parts[0]

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
