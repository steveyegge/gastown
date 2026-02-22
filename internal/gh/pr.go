// Package gh provides a wrapper for the GitHub CLI (gh) operations.
package gh

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PRState represents the state of a pull request.
type PRState string

const (
	PROpen    PRState = "OPEN"
	PRMerged  PRState = "MERGED"
	PRClosed  PRState = "CLOSED"
)

// PRClient provides GitHub PR operations.
type PRClient struct {
	// Repo is the repository in "owner/repo" format.
	// If empty, gh uses the current repository.
	Repo string
}

// NewPRClient creates a new PR client.
// If repo is empty, uses gh's default (current directory's repo).
func NewPRClient(repo string) *PRClient {
	return &PRClient{Repo: repo}
}

// CreatePROpts contains options for creating a PR.
type CreatePROpts struct {
	Title       string
	Body        string
	Base        string // Target branch (default: repo default branch)
	Head        string // Source branch (default: current branch)
	Draft       bool
	Web         bool   // Open in browser after creation
}

// CreatePR creates a new pull request.
// Returns the PR number and URL.
func (c *PRClient) CreatePR(opts CreatePROpts) (int, string, error) {
	args := []string{"pr", "create"}
	
	if c.Repo != "" {
		args = append(args, "--repo", c.Repo)
	}
	if opts.Title != "" {
		args = append(args, "--title", opts.Title)
	}
	if opts.Body != "" {
		args = append(args, "--body", opts.Body)
	}
	if opts.Base != "" {
		args = append(args, "--base", opts.Base)
	}
	if opts.Head != "" {
		args = append(args, "--head", opts.Head)
	}
	if opts.Draft {
		args = append(args, "--draft")
	}
	if opts.Web {
		args = append(args, "--web")
	}
	
	// Add --json to get structured output
	args = append(args, "--json", "number,url")
	
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return 0, "", fmt.Errorf("gh pr create failed: %s", string(exitErr.Stderr))
		}
		return 0, "", fmt.Errorf("gh pr create failed: %w", err)
	}
	
	// Parse JSON output
	var result struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, "", fmt.Errorf("parsing gh output: %w", err)
	}
	
	return result.Number, result.URL, nil
}

// GetPRState retrieves the state of a PR by number.
func (c *PRClient) GetPRState(prNumber int) (PRState, error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", prNumber), "--json", "state"}
	
	if c.Repo != "" {
		args = append(args, "--repo", c.Repo)
	}
	
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "Could not resolve") {
				return "", fmt.Errorf("PR #%d not found", prNumber)
			}
			return "", fmt.Errorf("gh pr view failed: %s", stderr)
		}
		return "", fmt.Errorf("gh pr view failed: %w", err)
	}
	
	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing gh output: %w", err)
	}
	
	return PRState(result.State), nil
}

// PROutput represents detailed PR information.
type PROutput struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	URL       string   `json:"url"`
	BaseRef   string   `json:"baseRefName"`
	HeadRef   string   `json:"headRefName"`
	IsDraft   bool     `json:"isDraft"`
	Labels    []string `json:"labels"`
}

// GetPR retrieves detailed information about a PR.
func (c *PRClient) GetPR(prNumber int) (*PROutput, error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", prNumber), 
		"--json", "number,title,state,url,baseRefName,headRefName,isDraft,labels"}
	
	if c.Repo != "" {
		args = append(args, "--repo", c.Repo)
	}
	
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr view failed: %w", err)
	}
	
	var result PROutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parsing gh output: %w", err)
	}
	
	return &result, nil
}

// ListPRs lists open PRs with optional head branch filter.
func (c *PRClient) ListPRs(headBranch string) ([]*PROutput, error) {
	args := []string{"pr", "list", "--state", "open", "--json", 
		"number,title,state,url,baseRefName,headRefName,isDraft,labels"}
	
	if c.Repo != "" {
		args = append(args, "--repo", c.Repo)
	}
	if headBranch != "" {
		args = append(args, "--head", headBranch)
	}
	
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}
	
	var results []*PROutput
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parsing gh output: %w", err)
	}
	
	return results, nil
}

// IsInstalled returns true if gh CLI is available in PATH.
func IsInstalled() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// CheckAuth verifies that gh is authenticated.
func CheckAuth() error {
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh not authenticated: %w", err)
	}
	return nil
}
