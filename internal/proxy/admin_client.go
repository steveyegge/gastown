package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AdminClient is an HTTP client for the proxy server's local admin API.
// It wraps the /v1/admin/* endpoints for cert lifecycle management.
// All methods are no-ops returning nil if the client is nil, making it
// safe to use without nil checks at call sites.
type AdminClient struct {
	baseURL string
	client  *http.Client
}

// NewAdminClient creates an AdminClient targeting the given admin address.
// adminAddr should be "host:port" (e.g. "127.0.0.1:9877").
func NewAdminClient(adminAddr string) *AdminClient {
	return &AdminClient{
		baseURL: "http://" + adminAddr,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IssueCertResult holds the response from a successful issue-cert call.
type IssueCertResult struct {
	CN        string `json:"cn"`
	Cert      string `json:"cert"`
	Key       string `json:"key"`
	CA        string `json:"ca"`
	Serial    string `json:"serial"`
	ExpiresAt string `json:"expires_at"`
}

// IssueCert requests a new polecat client certificate from the proxy CA.
// Returns nil, nil if the client is nil (proxy not running).
func (c *AdminClient) IssueCert(ctx context.Context, rig, name, ttl string) (*IssueCertResult, error) {
	if c == nil {
		return nil, nil
	}

	body, err := json.Marshal(issueCertRequest{
		Rig:  rig,
		Name: name,
		TTL:  ttl,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal issue-cert request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/admin/issue-cert", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create issue-cert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("issue-cert request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("issue-cert returned status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	var result IssueCertResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode issue-cert response: %w", err)
	}
	return &result, nil
}

// DenyCert revokes a certificate by its serial number (lowercase hex, no 0x prefix).
// Returns nil if the client is nil (proxy not running).
func (c *AdminClient) DenyCert(ctx context.Context, serial string) error {
	if c == nil {
		return nil
	}

	body, err := json.Marshal(denyCertRequest{Serial: serial})
	if err != nil {
		return fmt.Errorf("marshal deny-cert request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/admin/deny-cert", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create deny-cert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("deny-cert request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("deny-cert returned status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}

// Ping checks if the admin server is reachable by hitting the health endpoint.
// Returns nil if reachable and healthy (2xx), error otherwise. Returns nil if the client is nil.
func (c *AdminClient) Ping(ctx context.Context) error {
	if c == nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/admin/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ping returned status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}
