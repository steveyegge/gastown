// Package github provides GitHub integration via the gh CLI.
package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PR represents a GitHub pull request.
type PR struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	HeadRef string `json:"headRefName"`
	URL     string `json:"url"`
}

// CheckRun represents a CI check result for a PR.
type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`     // "completed", "in_progress", "queued", etc.
	Conclusion string `json:"conclusion"` // "success", "failure", "canceled", "timed_out", etc.
	URL        string `json:"link"`
}

// ghPRListEntry is the raw JSON structure from gh pr list --json.
type ghPRListEntry struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	URL         string `json:"url"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
}

// ListOpenPRs returns open PRs for the repo, optionally filtered by author.
// If authors is empty, all open PRs are returned.
func ListOpenPRs(repo string, authors []string) ([]PR, error) {
	args := []string{"pr", "list",
		"--repo", repo,
		"--state", "open",
		"--json", "number,title,author,headRefName,url",
		"--limit", "100",
	}
	out, err := runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("listing open PRs: %w", err)
	}

	var entries []ghPRListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing PR list: %w", err)
	}

	authorSet := make(map[string]bool, len(authors))
	for _, a := range authors {
		authorSet[strings.ToLower(a)] = true
	}

	var prs []PR
	for _, e := range entries {
		if len(authorSet) > 0 && !authorSet[strings.ToLower(e.Author.Login)] {
			continue
		}
		prs = append(prs, PR{
			Number:  e.Number,
			Title:   e.Title,
			Author:  e.Author.Login,
			HeadRef: e.HeadRefName,
			URL:     e.URL,
		})
	}
	return prs, nil
}

// ghCheckEntry is the raw JSON structure from gh pr checks --json.
type ghCheckEntry struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Conclusion string `json:"conclusion"`
	Link       string `json:"link"`
}

// GetCheckRuns returns check runs for a specific PR.
func GetCheckRuns(repo string, prNumber int) ([]CheckRun, error) {
	args := []string{"pr", "checks",
		fmt.Sprintf("%d", prNumber),
		"--repo", repo,
		"--json", "name,state,conclusion,link",
	}
	out, err := runGH(args...)
	if err != nil {
		return nil, fmt.Errorf("getting checks for PR #%d: %w", prNumber, err)
	}

	var entries []ghCheckEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing checks for PR #%d: %w", prNumber, err)
	}

	runs := make([]CheckRun, len(entries))
	for i, e := range entries {
		runs[i] = CheckRun{
			Name:       e.Name,
			Status:     e.State,
			Conclusion: e.Conclusion,
			URL:        e.Link,
		}
	}
	return runs, nil
}

// IsFailed returns true if the check run has a failure conclusion.
func (c CheckRun) IsFailed() bool {
	switch c.Conclusion {
	case "failure", "timed_out", "canceled", "action_required":
		return true
	default:
		return false
	}
}

// DetectRepo parses the GitHub owner/repo from a git remote URL.
// Handles both HTTPS and SSH formats:
//
//	https://github.com/owner/repo.git
//	git@github.com:owner/repo.git
func DetectRepo(workDir string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("detecting git remote: %w", err)
	}
	return ParseGitHubRepo(strings.TrimSpace(string(out)))
}

// ParseGitHubRepo extracts owner/repo from a GitHub remote URL.
func ParseGitHubRepo(remoteURL string) (string, error) {
	url := remoteURL

	// SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(url, "git@github.com:") {
		url = strings.TrimPrefix(url, "git@github.com:")
		url = strings.TrimSuffix(url, ".git")
		return url, nil
	}

	// HTTPS format: https://github.com/owner/repo.git
	if strings.Contains(url, "github.com/") {
		idx := strings.Index(url, "github.com/")
		url = url[idx+len("github.com/"):]
		url = strings.TrimSuffix(url, ".git")
		return url, nil
	}

	return "", fmt.Errorf("not a GitHub remote: %s", remoteURL)
}

// runGH executes a gh CLI command and returns stdout.
func runGH(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh %s: %s", strings.Join(args[:2], " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args[:2], " "), err)
	}
	return out, nil
}

// CheckGHAvailable returns an error if the gh CLI is not installed or not authenticated.
func CheckGHAvailable() error {
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		// Check if gh is installed at all
		if _, lookErr := exec.LookPath("gh"); lookErr != nil {
			return fmt.Errorf("gh CLI not found. Install from: https://cli.github.com/")
		}
		return fmt.Errorf("gh CLI not authenticated. Run: gh auth login")
	}
	return nil
}
