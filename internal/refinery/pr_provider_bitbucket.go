package refinery

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/bitbucket"
	"github.com/steveyegge/gastown/internal/git"
)

// bitbucketPRProvider implements PRProvider using the Bitbucket Cloud REST API.
type bitbucketPRProvider struct {
	git       *git.Git
	workspace string
	repoSlug  string
}

func newBitbucketPRProvider(g *git.Git) (PRProvider, error) {
	remoteURL, err := g.RemoteURL("origin")
	if err != nil {
		return nil, fmt.Errorf("bitbucket provider: failed to get origin remote URL: %w", err)
	}
	workspace, repoSlug, err := bitbucket.ParseBitbucketRemote(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("bitbucket provider: %w", err)
	}
	return &bitbucketPRProvider{
		git:       g,
		workspace: workspace,
		repoSlug:  repoSlug,
	}, nil
}

func (p *bitbucketPRProvider) FindPRNumber(branch string) (int, error) {
	return p.git.FindBitbucketPRNumber(p.workspace, p.repoSlug, branch)
}

func (p *bitbucketPRProvider) IsPRApproved(prNumber int) (bool, error) {
	return p.git.IsBitbucketPRApproved(p.workspace, p.repoSlug, prNumber)
}

func (p *bitbucketPRProvider) MergePR(prNumber int, method string) (string, error) {
	// Map generic merge methods to Bitbucket strategy names.
	bbStrategy := method
	switch method {
	case "squash":
		bbStrategy = "squash"
	case "merge":
		bbStrategy = "merge_commit"
	case "rebase":
		bbStrategy = "fast_forward"
	}
	return p.git.BitbucketPRMerge(p.workspace, p.repoSlug, prNumber, bbStrategy)
}
