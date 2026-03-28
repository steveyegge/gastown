// Package github provides a GitHub API client for PR lifecycle management.
//
// It wraps the GitHub REST API v3 and GraphQL API v4 for operations needed
// by the Gas Town merge queue: creating draft PRs, managing reviews,
// converting drafts to ready, and merging.
//
// Authentication uses a GITHUB_TOKEN environment variable.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	defaultRESTBase    = "https://api.github.com"
	defaultGraphQLBase = "https://api.github.com/graphql"
)

// Client wraps HTTP interactions with GitHub's REST and GraphQL APIs.
type Client struct {
	httpClient  *http.Client
	token       string
	restBase    string
	graphqlBase string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the underlying HTTP client (useful for testing).
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// WithToken overrides the token (default: GITHUB_TOKEN env var).
func WithToken(t string) Option {
	return func(cl *Client) { cl.token = t }
}

// WithRESTBase overrides the REST API base URL (for testing).
func WithRESTBase(url string) Option {
	return func(cl *Client) { cl.restBase = url }
}

// WithGraphQLBase overrides the GraphQL API base URL (for testing).
func WithGraphQLBase(url string) Option {
	return func(cl *Client) { cl.graphqlBase = url }
}

// NewClient creates a GitHub API client.
// By default it reads GITHUB_TOKEN from the environment.
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		httpClient:  http.DefaultClient,
		token:       os.Getenv("GITHUB_TOKEN"),
		restBase:    defaultRESTBase,
		graphqlBase: defaultGraphQLBase,
	}
	for _, o := range opts {
		o(c)
	}
	if c.token == "" {
		return nil, fmt.Errorf("github: GITHUB_TOKEN is required (set env var or use WithToken)")
	}
	return c, nil
}

// restRequest makes an authenticated REST API request and decodes the JSON response.
func (c *Client) restRequest(ctx context.Context, method, path string, body any, result any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("github: marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	url := c.restBase + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("github: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("github: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("github: decode response: %w", err)
		}
	}
	return nil
}

// graphqlRequest makes an authenticated GraphQL request.
func (c *Client) graphqlRequest(ctx context.Context, query string, variables map[string]any, result any) error {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("github: marshal graphql: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlBase, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("github: create graphql request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: graphql: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("github: read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return &APIError{
			Method:     "POST",
			Path:       "/graphql",
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return fmt.Errorf("github: decode graphql response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("github: graphql: %s", gqlResp.Errors[0].Message)
	}

	if result != nil {
		if err := json.Unmarshal(gqlResp.Data, result); err != nil {
			return fmt.Errorf("github: decode graphql data: %w", err)
		}
	}
	return nil
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// APIError represents a non-2xx response from the GitHub API.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github: %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}
