package githubci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultWorkflow = "CI"
	defaultPushWait = 15 * time.Second
	defaultPoll     = 3 * time.Second
	defaultTimeout  = 20 * time.Minute
)

// WorkflowRun captures the subset of gh run metadata needed for assurance.
type WorkflowRun struct {
	DatabaseID int64  `json:"databaseId"`
	HeadSHA    string `json:"headSha"`
	HeadBranch string `json:"headBranch"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"url"`
	CreatedAt  string `json:"createdAt"`
}

// EnsureOptions configures CI assurance for a branch SHA.
type EnsureOptions struct {
	RepoDir      string
	Repo         string
	Workflow     string
	Branch       string
	SHA          string
	PushWait     time.Duration
	PollInterval time.Duration
	Timeout      time.Duration
	Output       io.Writer
	// NowFn overrides time.Now for testing. Leave nil to use time.Now.
	NowFn func() time.Time
}

// Runner executes external commands.
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

// Client ensures GitHub Actions runs exist and complete for a given SHA.
type Client struct {
	runner Runner
}

// New creates a client backed by the real gh/git binaries.
func New() *Client {
	return &Client{runner: execRunner{}}
}

// NewWithRunner creates a client with a custom runner for tests.
func NewWithRunner(r Runner) *Client {
	return &Client{runner: r}
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // trusted binaries and fixed args
	return cmd.CombinedOutput()
}

// CheckAuth verifies gh is authenticated.
func (c *Client) CheckAuth(ctx context.Context) error {
	_, err := c.runner.Run(ctx, "gh", "auth", "status")
	if err != nil {
		return fmt.Errorf("gh auth status: %w", err)
	}
	return nil
}

// EnsureBranchCI ensures a workflow run exists for the branch SHA and waits for completion.
func (c *Client) EnsureBranchCI(ctx context.Context, opts EnsureOptions) (*WorkflowRun, error) {
	repo, err := c.resolveRepo(ctx, opts)
	if err != nil {
		return nil, err
	}
	workflow := strings.TrimSpace(opts.Workflow)
	if workflow == "" {
		workflow = defaultWorkflow
	}
	pushWait := opts.PushWait
	if pushWait <= 0 {
		pushWait = defaultPushWait
	}
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPoll
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if strings.TrimSpace(opts.Branch) == "" || strings.TrimSpace(opts.SHA) == "" {
		return nil, fmt.Errorf("branch and sha are required for GitHub CI assurance")
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if opts.Output != nil {
		_, _ = fmt.Fprintf(opts.Output, "[github-ci] waiting for push-triggered %s run on %s@%s\n", workflow, opts.Branch, shortSHA(opts.SHA))
	}

	nowFn := opts.NowFn
	if nowFn == nil {
		nowFn = time.Now
	}
	pushDeadline := nowFn().Add(pushWait)
	for {
		run, err := c.FindRun(runCtx, repo, workflow, opts.Branch, opts.SHA, "push")
		if err != nil {
			return nil, err
		}
		if run != nil {
			return c.waitForRun(runCtx, repo, run, pollInterval, opts.Output)
		}
		if nowFn().After(pushDeadline) {
			break
		}
		select {
		case <-runCtx.Done():
			return nil, runCtx.Err()
		case <-time.After(pollInterval):
		}
	}

	if opts.Output != nil {
		_, _ = fmt.Fprintf(opts.Output, "[github-ci] no push run for %s@%s; dispatching workflow_dispatch fallback\n", opts.Branch, shortSHA(opts.SHA))
	}
	if _, err := c.runner.Run(runCtx, "gh", "workflow", "run", workflow, "--repo", repo, "--ref", opts.Branch); err != nil {
		return nil, fmt.Errorf("gh workflow run %s: %w", workflow, err)
	}

	for {
		run, err := c.FindRun(runCtx, repo, workflow, opts.Branch, opts.SHA)
		if err != nil {
			return nil, err
		}
		if run != nil {
			return c.waitForRun(runCtx, repo, run, pollInterval, opts.Output)
		}
		select {
		case <-runCtx.Done():
			return nil, fmt.Errorf("timed out waiting for workflow run on %s@%s", opts.Branch, shortSHA(opts.SHA))
		case <-time.After(pollInterval):
		}
	}
}

// FindRun returns the newest workflow run matching the workflow, branch, sha,
// and optional event set.
func (c *Client) FindRun(ctx context.Context, repo, workflow, branch, sha string, events ...string) (*WorkflowRun, error) {
	args := []string{
		"run", "list",
		"--repo", repo,
		"--workflow", workflow,
		"--branch", branch,
		"--limit", "20",
		"--json", "databaseId,headSha,headBranch,event,status,conclusion,url,createdAt",
	}
	out, err := c.runner.Run(ctx, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("gh run list: %w", err)
	}
	var runs []WorkflowRun
	if err := json.Unmarshal(out, &runs); err != nil {
		return nil, fmt.Errorf("parse gh run list: %w", err)
	}
	eventSet := make(map[string]bool, len(events))
	for _, event := range events {
		if event != "" {
			eventSet[event] = true
		}
	}
	var matches []WorkflowRun
	for _, run := range runs {
		if run.HeadSHA != sha || run.HeadBranch != branch {
			continue
		}
		if len(eventSet) > 0 && !eventSet[run.Event] {
			continue
		}
		matches = append(matches, run)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt > matches[j].CreatedAt
	})
	run := matches[0]
	return &run, nil
}

func (c *Client) waitForRun(ctx context.Context, repo string, run *WorkflowRun, pollInterval time.Duration, output io.Writer) (*WorkflowRun, error) {
	if run == nil {
		return nil, fmt.Errorf("nil workflow run")
	}
	for {
		current, err := c.viewRun(ctx, repo, run.DatabaseID)
		if err != nil {
			return nil, err
		}
		if output != nil {
			_, _ = fmt.Fprintf(output, "[github-ci] run %d status=%s conclusion=%s\n", current.DatabaseID, current.Status, current.Conclusion)
		}
		if strings.EqualFold(current.Status, "completed") {
			if strings.EqualFold(current.Conclusion, "success") {
				return current, nil
			}
			return current, fmt.Errorf("workflow run failed: %s (%s)", current.Conclusion, current.URL)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (c *Client) viewRun(ctx context.Context, repo string, id int64) (*WorkflowRun, error) {
	out, err := c.runner.Run(ctx, "gh", "run", "view", fmt.Sprintf("%d", id), "--repo", repo, "--json", "databaseId,headSha,headBranch,event,status,conclusion,url,createdAt")
	if err != nil {
		return nil, fmt.Errorf("gh run view %d: %w", id, err)
	}
	var run WorkflowRun
	if err := json.Unmarshal(out, &run); err != nil {
		return nil, fmt.Errorf("parse gh run view: %w", err)
	}
	return &run, nil
}

func (c *Client) resolveRepo(ctx context.Context, opts EnsureOptions) (string, error) {
	if strings.TrimSpace(opts.Repo) != "" {
		return strings.TrimSpace(opts.Repo), nil
	}
	if strings.TrimSpace(opts.RepoDir) == "" {
		return "", fmt.Errorf("repo or repoDir is required")
	}
	remoteURL, err := ResolveOriginURL(ctx, c.runner, opts.RepoDir)
	if err != nil {
		return "", err
	}
	repo, err := RepoFromRemoteURL(remoteURL)
	if err != nil {
		return "", err
	}
	return repo, nil
}

// ResolveOriginURL returns the origin URL for a git repo.
func ResolveOriginURL(ctx context.Context, runner Runner, repoDir string) (string, error) {
	out, err := runner.Run(ctx, "git", "-C", repoDir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RepoFromRemoteURL converts a GitHub remote URL into owner/repo form.
func RepoFromRemoteURL(remoteURL string) (string, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	remoteURL = strings.TrimSuffix(remoteURL, ".git")
	switch {
	case strings.HasPrefix(remoteURL, "git@github.com:"):
		return strings.TrimPrefix(remoteURL, "git@github.com:"), nil
	case strings.HasPrefix(remoteURL, "https://github.com/"):
		return strings.TrimPrefix(remoteURL, "https://github.com/"), nil
	case strings.HasPrefix(remoteURL, "ssh://git@github.com/"):
		return strings.TrimPrefix(remoteURL, "ssh://git@github.com/"), nil
	default:
		return "", fmt.Errorf("unsupported GitHub remote URL: %s", remoteURL)
	}
}

// IsGitHubRemoteURL reports whether the remote points at github.com.
func IsGitHubRemoteURL(remoteURL string) bool {
	_, err := RepoFromRemoteURL(remoteURL)
	return err == nil
}

// FindWorkflowFile resolves a workflow name to a file inside .github/workflows.
// The workflow may be either the file basename or the workflow "name:".
func FindWorkflowFile(repoRoot, workflow string) (string, error) {
	workflow = strings.TrimSpace(workflow)
	if workflow == "" {
		workflow = defaultWorkflow
	}
	glob := filepath.Join(repoRoot, ".github", "workflows", "*")
	files, err := filepath.Glob(glob)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		if base == workflow {
			return file, nil
		}
		data, err := os.ReadFile(file) //nolint:gosec // repo-local workflow file
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "\nname: "+workflow) || strings.HasPrefix(string(data), "name: "+workflow+"\n") {
			return file, nil
		}
	}
	return "", fmt.Errorf("workflow %q not found under .github/workflows", workflow)
}

// WorkflowSupportsDispatch reports whether the workflow file supports workflow_dispatch.
func WorkflowSupportsDispatch(workflowFile string) (bool, error) {
	data, err := os.ReadFile(workflowFile) //nolint:gosec // repo-local workflow file
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), "workflow_dispatch:"), nil
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
