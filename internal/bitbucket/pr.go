package bitbucket

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
)

// PRComment represents a single comment on a PR.
type PRComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	User      string `json:"user"`
	CreatedOn string `json:"created_on"`
	HTMLURL   string `json:"html_url"`
}

// PRResult holds the result of creating a PR.
type PRResult struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

// CreateDraftPR creates a draft pull request on Bitbucket Cloud.
func (c *Client) CreateDraftPR(ctx context.Context, workspace, repoSlug, source, destination, title, description string) (PRResult, error) {
	reqBody := map[string]any{
		"title":       title,
		"description": description,
		"source": map[string]any{
			"branch": map[string]any{"name": source},
		},
		"destination": map[string]any{
			"branch": map[string]any{"name": destination},
		},
		"draft": true,
	}
	var resp struct {
		ID    int `json:"id"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests", workspace, repoSlug)
	if err := c.restRequest(ctx, "POST", path, reqBody, &resp); err != nil {
		return PRResult{}, fmt.Errorf("create draft PR: %w", err)
	}
	return PRResult{ID: resp.ID, URL: resp.Links.HTML.Href}, nil
}

// UpdatePRDescription updates the description of an existing PR.
func (c *Client) UpdatePRDescription(ctx context.Context, workspace, repoSlug string, prID int, description string) error {
	reqBody := map[string]any{"description": description}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", workspace, repoSlug, prID)
	if err := c.restRequest(ctx, "PUT", path, reqBody, nil); err != nil {
		return fmt.Errorf("update PR description: %w", err)
	}
	return nil
}

// GetPRApprovalStatus returns the overall approval state of a PR based on
// the participants list. Bitbucket uses an approved boolean per participant.
func (c *Client) GetPRApprovalStatus(ctx context.Context, workspace, repoSlug string, prID int) (ReviewState, error) {
	var pr struct {
		Participants []struct {
			Role     string `json:"role"`
			Approved bool   `json:"approved"`
			State    string `json:"state"`
			User     struct {
				DisplayName string `json:"display_name"`
			} `json:"user"`
		} `json:"participants"`
	}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", workspace, repoSlug, prID)
	if err := c.restRequest(ctx, "GET", path, nil, &pr); err != nil {
		return "", fmt.Errorf("get PR approval status: %w", err)
	}

	if len(pr.Participants) == 0 {
		return ReviewPending, nil
	}

	hasApproval := false
	for _, p := range pr.Participants {
		if p.Role != "REVIEWER" {
			continue
		}
		if p.State == "changes_requested" {
			return ReviewChangesRequired, nil
		}
		if p.Approved {
			hasApproval = true
		}
	}
	if hasApproval {
		return ReviewApproved, nil
	}
	return ReviewPending, nil
}

// GetPRComments returns the comments on a PR.
func (c *Client) GetPRComments(ctx context.Context, workspace, repoSlug string, prID int) ([]PRComment, error) {
	var resp struct {
		Values []struct {
			ID      int64  `json:"id"`
			Content struct {
				Raw string `json:"raw"`
			} `json:"content"`
			Inline *struct {
				Path string `json:"path"`
				To   int    `json:"to"`
			} `json:"inline"`
			CreatedOn string `json:"created_on"`
			User      struct {
				DisplayName string `json:"display_name"`
			} `json:"user"`
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"values"`
	}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, prID)
	if err := c.restRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get PR comments: %w", err)
	}

	comments := make([]PRComment, len(resp.Values))
	for i, v := range resp.Values {
		c := PRComment{
			ID:        v.ID,
			Body:      v.Content.Raw,
			User:      v.User.DisplayName,
			CreatedOn: v.CreatedOn,
			HTMLURL:   v.Links.HTML.Href,
		}
		if v.Inline != nil {
			c.Path = v.Inline.Path
			c.Line = v.Inline.To
		}
		comments[i] = c
	}
	return comments, nil
}

// ReplyToPRComment posts a reply to an existing comment on a PR.
func (c *Client) ReplyToPRComment(ctx context.Context, workspace, repoSlug string, prID int, parentID int64, body string) error {
	reqBody := map[string]any{
		"content": map[string]any{"raw": body},
		"parent":  map[string]any{"id": parentID},
	}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, prID)
	if err := c.restRequest(ctx, "POST", path, reqBody, nil); err != nil {
		return fmt.Errorf("reply to PR comment: %w", err)
	}
	return nil
}

// MergePR merges a pull request using the specified strategy.
// Valid strategies: "merge_commit", "squash", "fast_forward".
func (c *Client) MergePR(ctx context.Context, workspace, repoSlug string, prID int, strategy string) error {
	reqBody := map[string]any{
		"merge_strategy":       strategy,
		"close_source_branch": true,
	}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/merge", workspace, repoSlug, prID)
	if err := c.restRequest(ctx, "POST", path, reqBody, nil); err != nil {
		return fmt.Errorf("merge PR: %w", err)
	}
	return nil
}

// GetRepoMergeStrategies returns the preferred merge strategy for a repository.
// It inspects the branching model and returns "squash", "fast_forward", or "merge_commit"
// (preferring squash > fast_forward > merge_commit).
func (c *Client) GetRepoMergeStrategies(ctx context.Context, workspace, repoSlug string) (string, error) {
	var branchingModel struct {
		Development struct {
			MergeStrategy string `json:"merge_strategy"`
		} `json:"development"`
	}
	path := fmt.Sprintf("/repositories/%s/%s/branching-model", workspace, repoSlug)
	if err := c.restRequest(ctx, "GET", path, nil, &branchingModel); err != nil {
		// Fallback: if branching model is not configured, default to squash.
		return "squash", nil
	}

	switch branchingModel.Development.MergeStrategy {
	case "squash", "fast_forward", "merge_commit":
		return branchingModel.Development.MergeStrategy, nil
	default:
		return "squash", nil
	}
}
