// Package github implements the GitHub Issues integration for faultline.
// It creates, updates, and closes GitHub issues in response to faultline
// lifecycle events (new issue, resolved, regression).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/outdoorsea/faultline/internal/integrations"
	"github.com/outdoorsea/faultline/internal/notify"
)

func init() {
	integrations.Register(integrations.TypeGitHubIssues, func(config json.RawMessage) (integrations.Integration, error) {
		var cfg Config
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("github_issues: parse config: %w", err)
		}
		if err := cfg.validate(); err != nil {
			return nil, fmt.Errorf("github_issues: %w", err)
		}
		return New(cfg, nil), nil
	})
}

// Config holds the per-project GitHub Issues integration settings.
type Config struct {
	Owner      string            `json:"owner"`       // GitHub repo owner (user or org)
	Repo       string            `json:"repo"`        // GitHub repo name
	Token      string            `json:"token"`       // Personal access token or app token
	BaseURL    string            `json:"base_url"`    // Faultline dashboard URL for backlinks
	APIBaseURL string            `json:"api_base_url"` // Override GitHub API base (for GHE), default https://api.github.com
	Labels     map[string]string `json:"labels"`      // faultline level → GitHub label mapping
}

func (c *Config) validate() error {
	if c.Owner == "" {
		return fmt.Errorf("owner is required")
	}
	if c.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

func (c *Config) apiBase() string {
	if c.APIBaseURL != "" {
		return strings.TrimSuffix(c.APIBaseURL, "/")
	}
	return "https://api.github.com"
}

// HTTPClient is the interface for HTTP calls, allowing test injection.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// GitHubIssues implements integrations.Integration for GitHub Issues.
type GitHubIssues struct {
	cfg    Config
	client HTTPClient
}

// New creates a GitHubIssues integration. If client is nil, a default HTTP
// client with a 15-second timeout is used.
func New(cfg Config, client HTTPClient) *GitHubIssues {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &GitHubIssues{cfg: cfg, client: client}
}

func (g *GitHubIssues) Type() integrations.IntegrationType {
	return integrations.TypeGitHubIssues
}

// OnNewIssue creates a GitHub issue for a new faultline issue group.
func (g *GitHubIssues) OnNewIssue(ctx context.Context, event notify.Event) error {
	body := g.buildBody(event)
	labels := g.labelsFor(event)

	payload := map[string]any{
		"title":  fmt.Sprintf("[Faultline] %s", event.Title),
		"body":   body,
		"labels": labels,
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues", g.cfg.apiBase(), g.cfg.Owner, g.cfg.Repo)
	_, err := g.doAPI(ctx, http.MethodPost, url, payload)
	return err
}

// OnResolved adds a comment and closes the matching GitHub issue.
func (g *GitHubIssues) OnResolved(ctx context.Context, event notify.Event) error {
	issueNumber, err := g.findIssue(ctx, event.GroupID)
	if err != nil {
		return err
	}
	if issueNumber == 0 {
		return nil // no matching issue found, nothing to do
	}

	comment := fmt.Sprintf("Resolved in Faultline.%s", g.faultlineLink(event))
	if _, err := g.doAPI(ctx, http.MethodPost,
		fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", g.cfg.apiBase(), g.cfg.Owner, g.cfg.Repo, issueNumber),
		map[string]any{"body": comment}); err != nil {
		return fmt.Errorf("add resolved comment: %w", err)
	}

	if _, err := g.doAPI(ctx, http.MethodPatch,
		fmt.Sprintf("%s/repos/%s/%s/issues/%d", g.cfg.apiBase(), g.cfg.Owner, g.cfg.Repo, issueNumber),
		map[string]any{"state": "closed", "state_reason": "completed"}); err != nil {
		return fmt.Errorf("close issue: %w", err)
	}

	return nil
}

// OnRegression reopens the GitHub issue and adds a regression comment.
func (g *GitHubIssues) OnRegression(ctx context.Context, event notify.Event) error {
	issueNumber, err := g.findIssue(ctx, event.GroupID)
	if err != nil {
		return err
	}
	if issueNumber == 0 {
		// No existing issue — create a new one tagged as regression.
		return g.OnNewIssue(ctx, event)
	}

	comment := fmt.Sprintf("Regression detected in Faultline. This issue has been reopened.%s", g.faultlineLink(event))
	if _, err := g.doAPI(ctx, http.MethodPost,
		fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", g.cfg.apiBase(), g.cfg.Owner, g.cfg.Repo, issueNumber),
		map[string]any{"body": comment}); err != nil {
		return fmt.Errorf("add regression comment: %w", err)
	}

	if _, err := g.doAPI(ctx, http.MethodPatch,
		fmt.Sprintf("%s/repos/%s/%s/issues/%d", g.cfg.apiBase(), g.cfg.Owner, g.cfg.Repo, issueNumber),
		map[string]any{"state": "open"}); err != nil {
		return fmt.Errorf("reopen issue: %w", err)
	}

	return nil
}

// findIssue searches for an open or closed GitHub issue matching the faultline group ID.
func (g *GitHubIssues) findIssue(ctx context.Context, groupID string) (int, error) {
	if groupID == "" {
		return 0, nil
	}
	// Search by the marker we embed in issue bodies.
	query := fmt.Sprintf("repo:%s/%s \"%s\" in:body", g.cfg.Owner, g.cfg.Repo, faultlineMarker(groupID))
	url := fmt.Sprintf("%s/search/issues?q=%s", g.cfg.apiBase(), urlEncode(query))

	resp, err := g.doAPI(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("search issues: %w", err)
	}

	var result struct {
		Items []struct {
			Number int `json:"number"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, fmt.Errorf("parse search response: %w", err)
	}
	if len(result.Items) == 0 {
		return 0, nil
	}
	return result.Items[0].Number, nil
}

func (g *GitHubIssues) buildBody(event notify.Event) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**%s** detected by [Faultline](https://github.com/outdoorsea/faultline)\n\n", severityName(event.Level)))

	b.WriteString("| Field | Value |\n|-------|-------|\n")
	b.WriteString(fmt.Sprintf("| Level | `%s` |\n", event.Level))
	if event.Culprit != "" {
		b.WriteString(fmt.Sprintf("| Culprit | `%s` |\n", event.Culprit))
	}
	if event.Platform != "" {
		b.WriteString(fmt.Sprintf("| Platform | %s |\n", event.Platform))
	}
	b.WriteString(fmt.Sprintf("| Events | %d |\n", event.EventCount))

	link := g.faultlineLink(event)
	if link != "" {
		b.WriteString(fmt.Sprintf("\n%s\n", link))
	}

	b.WriteString(fmt.Sprintf("\n---\n<!-- %s -->", faultlineMarker(event.GroupID)))

	return b.String()
}

func (g *GitHubIssues) faultlineLink(event notify.Event) string {
	if g.cfg.BaseURL == "" || event.GroupID == "" {
		return ""
	}
	return fmt.Sprintf("[View in Faultline](%s/api/%d/issues/%s)", strings.TrimSuffix(g.cfg.BaseURL, "/"), event.ProjectID, event.GroupID)
}

func (g *GitHubIssues) labelsFor(event notify.Event) []string {
	labels := []string{"faultline"}
	if g.cfg.Labels != nil {
		if mapped, ok := g.cfg.Labels[event.Level]; ok {
			labels = append(labels, mapped)
		}
	}
	return labels
}

// doAPI makes an authenticated GitHub API request and returns the response body.
func (g *GitHubIssues) doAPI(ctx context.Context, method, url string, payload any) (json.RawMessage, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github api %s %d: %s", method, resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

func faultlineMarker(groupID string) string {
	return fmt.Sprintf("faultline-group-id:%s", groupID)
}

func severityName(level string) string {
	switch level {
	case "fatal":
		return "Critical error"
	case "error":
		return "Error"
	case "warning":
		return "Warning"
	default:
		return "Issue"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func urlEncode(s string) string {
	// Minimal encoding for search query parameter.
	r := strings.NewReplacer(
		" ", "+",
		"\"", "%22",
		"#", "%23",
	)
	return r.Replace(s)
}
