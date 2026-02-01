// Package epic provides upstream contribution workflow support.
package epic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GHClient defines the interface for GitHub CLI operations.
// This allows injecting a stub for testing.
type GHClient interface {
	// GetPRChecks returns CI check results for a PR.
	GetPRChecks(workDir string, prNumber int) ([]PRCheck, error)

	// GetPRReviews returns review information for a PR.
	GetPRReviews(workDir string, prNumber int) (*PRReviewInfo, error)
}

// PRCheck represents a single CI check result.
type PRCheck struct {
	State      string `json:"state"`
	Name       string `json:"name"`
	DetailsURL string `json:"detailsUrl"`
}

// PRReviewInfo contains review decision and individual reviews.
type PRReviewInfo struct {
	ReviewDecision string     `json:"reviewDecision"`
	Reviews        []PRReview `json:"reviews"`
}

// PRReview represents a single review.
type PRReview struct {
	State string `json:"state"`
}

// DefaultGHClient is the default implementation that uses the real gh CLI.
var DefaultGHClient GHClient = &RealGHClient{}

// RealGHClient implements GHClient using the actual gh CLI.
type RealGHClient struct{}

// GetPRChecks runs gh pr checks and returns the results.
func (c *RealGHClient) GetPRChecks(workDir string, prNumber int) ([]PRCheck, error) {
	cmd := exec.Command("gh", "pr", "checks", fmt.Sprintf("%d", prNumber), "--json", "state,name,detailsUrl")
	cmd.Dir = workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("getting PR checks: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	var checks []PRCheck
	if err := json.Unmarshal(stdout.Bytes(), &checks); err != nil {
		return nil, fmt.Errorf("parsing PR checks: %w", err)
	}

	return checks, nil
}

// GetPRReviews runs gh pr view and returns review information.
func (c *RealGHClient) GetPRReviews(workDir string, prNumber int) (*PRReviewInfo, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "reviewDecision,reviews")
	cmd.Dir = workDir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("getting PR review status: %w", err)
	}

	var info PRReviewInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("parsing PR review status: %w", err)
	}

	return &info, nil
}

// StubGHClient is a test stub that returns pre-configured responses.
type StubGHClient struct {
	// ChecksResponse is returned by GetPRChecks if ChecksError is nil.
	ChecksResponse []PRCheck
	// ChecksError is returned by GetPRChecks if set.
	ChecksError error

	// ReviewsResponse is returned by GetPRReviews if ReviewsError is nil.
	ReviewsResponse *PRReviewInfo
	// ReviewsError is returned by GetPRReviews if set.
	ReviewsError error

	// CallLog records all calls for verification.
	CallLog []GHClientCall
}

// GHClientCall records a single call to the GH client.
type GHClientCall struct {
	Method   string
	WorkDir  string
	PRNumber int
}

// GetPRChecks returns the stubbed response.
func (c *StubGHClient) GetPRChecks(workDir string, prNumber int) ([]PRCheck, error) {
	c.CallLog = append(c.CallLog, GHClientCall{
		Method:   "GetPRChecks",
		WorkDir:  workDir,
		PRNumber: prNumber,
	})
	if c.ChecksError != nil {
		return nil, c.ChecksError
	}
	return c.ChecksResponse, nil
}

// GetPRReviews returns the stubbed response.
func (c *StubGHClient) GetPRReviews(workDir string, prNumber int) (*PRReviewInfo, error) {
	c.CallLog = append(c.CallLog, GHClientCall{
		Method:   "GetPRReviews",
		WorkDir:  workDir,
		PRNumber: prNumber,
	})
	if c.ReviewsError != nil {
		return nil, c.ReviewsError
	}
	return c.ReviewsResponse, nil
}

// GetPRCIStatusWithClient gets CI status using the provided GH client.
func GetPRCIStatusWithClient(client GHClient, workDir string, prNumber int) (*CIStatus, error) {
	checks, err := client.GetPRChecks(workDir, prNumber)
	if err != nil {
		return nil, err
	}

	return aggregateCIStatus(prNumber, checks), nil
}

// GetPRReviewStatusWithClient gets review status using the provided GH client.
func GetPRReviewStatusWithClient(client GHClient, workDir string, prNumber int) (string, int, error) {
	info, err := client.GetPRReviews(workDir, prNumber)
	if err != nil {
		return "", 0, err
	}

	return parseReviewStatus(info)
}

// aggregateCIStatus converts raw check data into a CIStatus.
func aggregateCIStatus(prNumber int, checks []PRCheck) *CIStatus {
	status := &CIStatus{
		PRNumber: prNumber,
		State:    "success",
	}

	var failedChecks []string
	var pendingChecks []string

	for _, check := range checks {
		switch check.State {
		case "FAILURE", "ERROR":
			status.State = "failure"
			failedChecks = append(failedChecks, check.Name)
			if status.URL == "" {
				status.URL = check.DetailsURL
			}
		case "PENDING", "QUEUED", "IN_PROGRESS":
			if status.State != "failure" {
				status.State = "pending"
			}
			pendingChecks = append(pendingChecks, check.Name)
		}
	}

	if len(failedChecks) > 0 {
		status.Details = fmt.Sprintf("Failed: %s", strings.Join(failedChecks, ", "))
	} else if len(pendingChecks) > 0 {
		status.Details = fmt.Sprintf("Pending: %s", strings.Join(pendingChecks, ", "))
	} else {
		status.Details = "All checks passed"
	}

	return status
}

// parseReviewStatus converts review info into status and approval count.
func parseReviewStatus(info *PRReviewInfo) (string, int, error) {
	// Count approvals
	approvals := 0
	for _, review := range info.Reviews {
		if review.State == "APPROVED" {
			approvals++
		}
	}

	// Map review decision to status
	status := "pending"
	switch info.ReviewDecision {
	case "APPROVED":
		status = PRStatusApproved
	case "CHANGES_REQUESTED":
		status = PRStatusChangesRequested
	case "REVIEW_REQUIRED":
		status = "review_required"
	}

	return status, approvals, nil
}
