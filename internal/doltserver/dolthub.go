package doltserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// dolthubAPIBase is the DoltHub REST API base URL.
// It is a var (not const) so tests can override it with httptest servers.
// Not safe for parallel tests — tests that mutate this must not use t.Parallel().
var dolthubAPIBase = "https://www.dolthub.com/api/v1alpha1"

const (
	// dolthubRemoteBase is the Dolt remote API endpoint for push/pull.
	dolthubRemoteBase = "https://doltremoteapi.dolthub.com"
)

// DoltHubToken returns the DoltHub API token from the environment.
// Returns empty string if not configured.
func DoltHubToken() string {
	return os.Getenv("DOLTHUB_TOKEN")
}

// DoltHubOrg returns the default DoltHub organization from the environment.
// Returns empty string if not configured.
func DoltHubOrg() string {
	return os.Getenv("DOLTHUB_ORG")
}

// DoltHubRepoName converts a local database name to a DoltHub repo name.
// Replaces underscores with hyphens (e.g., "beads_gt" → "beads-gt").
// Special case: "hq" maps to "gt-hq" (the town-level HQ database uses the gt- prefix).
func DoltHubRepoName(dbName string) string {
	if dbName == "hq" {
		return "gt-hq"
	}
	return strings.ReplaceAll(dbName, "_", "-")
}

// DoltHubRemoteURL returns the full Dolt remote URL for a DoltHub repo.
func DoltHubRemoteURL(org, repo string) string {
	return fmt.Sprintf("%s/%s/%s", dolthubRemoteBase, org, repo)
}

// CreateDoltHubRepo creates a private repository on DoltHub via the API.
// Returns nil if the repo was created or already exists.
func CreateDoltHubRepo(org, repo, token string) error {
	body := map[string]string{
		"ownerName":  org,
		"repoName":   repo,
		"visibility": "private",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", dolthubAPIBase+"/database", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authorization", "token "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DoltHub API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 200 = created, 409 or similar = already exists (both are fine)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Parse error response for better messaging
	var errResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&errResp); decErr == nil {
		// "already exists" is not an error for our purposes
		if strings.Contains(strings.ToLower(errResp.Message), "already exists") {
			return nil
		}
		return fmt.Errorf("DoltHub API error (HTTP %d): %s", resp.StatusCode, errResp.Message)
	}
	return fmt.Errorf("DoltHub API error (HTTP %d)", resp.StatusCode)
}

// AddRemote adds a DoltHub origin remote to a local Dolt database directory.
// Skips if an origin remote already exists.
func AddRemote(dbDir, org, repo string) error {
	// Check if origin already exists
	existing, err := HasRemote(dbDir)
	if err != nil {
		return err
	}
	if existing != "" {
		return nil // Already has a remote
	}

	url := DoltHubRemoteURL(org, repo)
	cmd := exec.Command("dolt", "remote", "add", "origin", url)
	cmd.Dir = dbDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		// "already exists" is fine
		if strings.Contains(strings.ToLower(msg), "already exists") {
			return nil
		}
		return fmt.Errorf("dolt remote add: %w (%s)", err, msg)
	}
	return nil
}

// SetupDoltHubRemote creates a DoltHub repo, adds the remote, and does an
// initial push. Each step is fail-fast — the function returns on the first
// error because each step requires the previous to succeed (can't add a
// remote if repo creation failed, can't push if the remote wasn't added).
func SetupDoltHubRemote(dbDir, org, dbName, token string) error {
	repo := DoltHubRepoName(dbName)

	// Step 1: Create the DoltHub repo
	if err := CreateDoltHubRepo(org, repo, token); err != nil {
		return fmt.Errorf("creating DoltHub repo %s/%s: %w", org, repo, err)
	}

	// Step 2: Add the remote locally
	if err := AddRemote(dbDir, org, repo); err != nil {
		return fmt.Errorf("adding remote for %s/%s: %w", org, repo, err)
	}

	// Step 3: Initial push
	if err := PushDatabase(dbDir, false); err != nil {
		return fmt.Errorf("initial push to %s/%s: %w", org, repo, err)
	}

	return nil
}
