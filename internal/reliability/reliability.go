package reliability

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	gitpkg "github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/githubci"
	"github.com/steveyegge/gastown/internal/verify"
)

// RigContext describes the effective overnight-reliability contract for a rig clone.
type RigContext struct {
	RigPath   string
	RepoRoot  string
	RemoteURL string
	Settings  *config.RigSettings
	GitHubCI  *config.GitHubCIConfig
}

// LoadRigContext resolves effective rig settings and GitHub CI policy.
func LoadRigContext(rigPath, repoRoot string) (*RigContext, error) {
	settings, err := config.LoadEffectiveRigSettings(rigPath, repoRoot)
	if err != nil {
		return nil, err
	}

	remoteURL := ""
	if strings.TrimSpace(repoRoot) != "" {
		if url, err := gitpkg.NewGit(repoRoot).RemoteURL("origin"); err == nil {
			remoteURL = strings.TrimSpace(url)
		}
	}

	return &RigContext{
		RigPath:   rigPath,
		RepoRoot:  repoRoot,
		RemoteURL: remoteURL,
		Settings:  settings,
		GitHubCI:  config.EffectiveGitHubCIForRemote(settings, remoteURL),
	}, nil
}

// ValidateStrictPreconditions enforces strict-mode repo contract requirements.
func (r *RigContext) ValidateStrictPreconditions() error {
	if r == nil {
		return nil
	}
	return config.ValidateStrictRepoContract(r.Settings)
}

// RunVerificationPhase executes repo-configured verification gates for the phase.
func (r *RigContext) RunVerificationPhase(ctx context.Context, phase verify.Phase, out io.Writer, parallel bool) (verify.Summary, error) {
	if r == nil || r.Settings == nil || r.Settings.MergeQueue == nil {
		return verify.Summary{Success: true}, nil
	}
	gates := r.Settings.MergeQueue.ToVerifyGates()
	summary := verify.RunPhase(ctx, r.RepoRoot, gates, phase, verify.RunOptions{
		Parallel: parallel,
		Output:   out,
	})
	if summary.Success {
		return summary, nil
	}
	var failures []string
	for _, result := range summary.Results {
		if !result.Success {
			failures = append(failures, result.Name)
		}
	}
	return summary, fmt.Errorf("verification gates failed: %s", strings.Join(failures, ", "))
}

// EnsureGitHubBranchCI waits for a workflow run for the branch SHA, dispatching
// workflow_dispatch when push-triggered CI does not appear.
func (r *RigContext) EnsureGitHubBranchCI(ctx context.Context, branch, sha string, out io.Writer) (*githubci.WorkflowRun, error) {
	if r == nil || r.GitHubCI == nil || !r.GitHubCI.IsRequired() {
		return nil, nil
	}
	client := githubci.New()
	if err := client.CheckAuth(ctx); err != nil {
		return nil, err
	}
	return client.EnsureBranchCI(ctx, githubci.EnsureOptions{
		RepoDir:  r.RepoRoot,
		Workflow: r.GitHubCI.WorkflowName(),
		Branch:   branch,
		SHA:      sha,
		Output:   out,
	})
}
