package github

import (
	"context"
	"fmt"
)

// ReviewState represents the overall review status of a PR.
type ReviewState string

const (
	ReviewPending         ReviewState = "PENDING"
	ReviewApproved        ReviewState = "APPROVED"
	ReviewChangesRequired ReviewState = "CHANGES_REQUESTED"
	ReviewCommented       ReviewState = "COMMENTED"
	ReviewDismissed       ReviewState = "DISMISSED"
)

// ReviewComment represents a single review comment on a PR.
type ReviewComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	User      string `json:"user"`
	CreatedAt string `json:"created_at"`
	HTMLURL   string `json:"html_url"`
}

// PRResult holds the result of creating a PR.
type PRResult struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
}

// CreateDraftPR creates a draft pull request.
// For cross-repo (fork) PRs, set head to "forkOwner:branchName".
func (c *Client) CreateDraftPR(ctx context.Context, owner, repo, head, base, title, body string) (PRResult, error) {
	reqBody := map[string]any{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
		"draft": true,
	}
	var resp struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	if err := c.restRequest(ctx, "POST", path, reqBody, &resp); err != nil {
		return PRResult{}, fmt.Errorf("create draft PR: %w", err)
	}
	return PRResult{Number: resp.Number, URL: resp.HTMLURL}, nil
}

// UpdatePRDescription updates the body of an existing PR.
func (c *Client) UpdatePRDescription(ctx context.Context, owner, repo string, prNumber int, body string) error {
	reqBody := map[string]any{"body": body}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	if err := c.restRequest(ctx, "PATCH", path, reqBody, nil); err != nil {
		return fmt.Errorf("update PR description: %w", err)
	}
	return nil
}

// ConvertDraftToReady marks a draft PR as ready for review.
// This uses the GraphQL API because REST does not support this operation.
func (c *Client) ConvertDraftToReady(ctx context.Context, owner, repo string, prNumber int) error {
	// First, get the PR's GraphQL node ID via REST.
	nodeID, err := c.getPRNodeID(ctx, owner, repo, prNumber)
	if err != nil {
		return fmt.Errorf("convert draft to ready: %w", err)
	}

	const mutation = `mutation($id: ID!) {
		markPullRequestReadyForReview(input: {pullRequestId: $id}) {
			pullRequest { id }
		}
	}`
	vars := map[string]any{"id": nodeID}
	if err := c.graphqlRequest(ctx, mutation, vars, nil); err != nil {
		return fmt.Errorf("convert draft to ready: %w", err)
	}
	return nil
}

// GetPRReviewStatus returns the overall review state of a PR based on the
// most recent review from each reviewer.
func (c *Client) GetPRReviewStatus(ctx context.Context, owner, repo string, prNumber int) (ReviewState, error) {
	var reviews []struct {
		State string `json:"state"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)
	if err := c.restRequest(ctx, "GET", path, nil, &reviews); err != nil {
		return "", fmt.Errorf("get PR review status: %w", err)
	}

	if len(reviews) == 0 {
		return ReviewPending, nil
	}

	// Collect the latest review state per reviewer.
	latest := make(map[string]string)
	for _, r := range reviews {
		latest[r.User.Login] = r.State
	}

	// If any reviewer requested changes, overall state is CHANGES_REQUESTED.
	// Otherwise, if any approved, it's APPROVED.
	hasApproval := false
	for _, state := range latest {
		switch ReviewState(state) {
		case ReviewChangesRequired:
			return ReviewChangesRequired, nil
		case ReviewApproved:
			hasApproval = true
		}
	}
	if hasApproval {
		return ReviewApproved, nil
	}
	return ReviewPending, nil
}

// GetPRReviewComments returns the review comments on a PR.
func (c *Client) GetPRReviewComments(ctx context.Context, owner, repo string, prNumber int) ([]ReviewComment, error) {
	var raw []struct {
		ID        int64  `json:"id"`
		Body      string `json:"body"`
		Path      string `json:"path"`
		Line      int    `json:"line"`
		CreatedAt string `json:"created_at"`
		HTMLURL   string `json:"html_url"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, repo, prNumber)
	if err := c.restRequest(ctx, "GET", path, nil, &raw); err != nil {
		return nil, fmt.Errorf("get PR review comments: %w", err)
	}

	comments := make([]ReviewComment, len(raw))
	for i, r := range raw {
		comments[i] = ReviewComment{
			ID:        r.ID,
			Body:      r.Body,
			Path:      r.Path,
			Line:      r.Line,
			User:      r.User.Login,
			CreatedAt: r.CreatedAt,
			HTMLURL:   r.HTMLURL,
		}
	}
	return comments, nil
}

// ReplyToPRComment posts a reply to an existing review comment.
func (c *Client) ReplyToPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64, body string) error {
	reqBody := map[string]any{
		"body":         body,
		"in_reply_to":  commentID,
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", owner, repo, prNumber)
	if err := c.restRequest(ctx, "POST", path, reqBody, nil); err != nil {
		return fmt.Errorf("reply to PR comment: %w", err)
	}
	return nil
}

// MergePR merges a pull request using the specified method.
// Valid methods: "merge", "squash", "rebase".
func (c *Client) MergePR(ctx context.Context, owner, repo string, prNumber int, method string) error {
	reqBody := map[string]any{"merge_method": method}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, prNumber)
	if err := c.restRequest(ctx, "PUT", path, reqBody, nil); err != nil {
		return fmt.Errorf("merge PR: %w", err)
	}
	return nil
}

// GetRepoMergeMethod returns the preferred merge method for a repository.
// It inspects the repo settings and returns "merge", "squash", or "rebase"
// based on which methods are allowed (preferring squash > rebase > merge).
func (c *Client) GetRepoMergeMethod(ctx context.Context, owner, repo string) (string, error) {
	var repoInfo struct {
		AllowMergeCommit bool `json:"allow_merge_commit"`
		AllowSquashMerge bool `json:"allow_squash_merge"`
		AllowRebaseMerge bool `json:"allow_rebase_merge"`
	}
	path := fmt.Sprintf("/repos/%s/%s", owner, repo)
	if err := c.restRequest(ctx, "GET", path, nil, &repoInfo); err != nil {
		return "", fmt.Errorf("get repo merge method: %w", err)
	}

	switch {
	case repoInfo.AllowSquashMerge:
		return "squash", nil
	case repoInfo.AllowRebaseMerge:
		return "rebase", nil
	case repoInfo.AllowMergeCommit:
		return "merge", nil
	default:
		return "", fmt.Errorf("github: no merge methods enabled for %s/%s", owner, repo)
	}
}

// getPRNodeID fetches the GraphQL node ID for a PR (needed for mutations).
func (c *Client) getPRNodeID(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	var pr struct {
		NodeID string `json:"node_id"`
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	if err := c.restRequest(ctx, "GET", path, nil, &pr); err != nil {
		return "", fmt.Errorf("get PR node ID: %w", err)
	}
	if pr.NodeID == "" {
		return "", fmt.Errorf("github: PR %d has no node_id", prNumber)
	}
	return pr.NodeID, nil
}
