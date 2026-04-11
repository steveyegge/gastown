package refinery

import "github.com/steveyegge/gastown/internal/git"

// githubPRProvider implements PRProvider using the gh CLI via git.Git.
type githubPRProvider struct {
	git *git.Git
}

func newGitHubPRProvider(g *git.Git) PRProvider {
	return &githubPRProvider{git: g}
}

func (p *githubPRProvider) FindPRNumber(branch string) (int, error) {
	return p.git.FindPRNumber(branch)
}

func (p *githubPRProvider) IsPRApproved(prNumber int) (bool, error) {
	return p.git.IsPRApproved(prNumber)
}

func (p *githubPRProvider) MergePR(prNumber int, method string) (string, error) {
	return p.git.GhPrMerge(prNumber, method)
}
