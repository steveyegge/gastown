// Package daemonclient queries the BD Daemon for agent bead state.
package daemonclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AgentBead represents an active agent bead from the daemon.
type AgentBead struct {
	// ID is the bead identifier (e.g., "hq-mayor", "gastown-crew-k8s").
	ID string

	// Rig is the rig name (e.g., "town", "gastown").
	Rig string

	// Role is the agent role (e.g., "mayor", "crew", "polecat").
	Role string

	// AgentName is the agent's name within its role (e.g., "hq", "k8s").
	AgentName string

	// Image overrides the default agent image. Empty means use default.
	Image string

	// Metadata contains additional bead metadata from the daemon.
	Metadata map[string]string
}

// BeadLister lists active agent beads from the daemon.
type BeadLister interface {
	ListAgentBeads(ctx context.Context) ([]AgentBead, error)
}

// Config for the daemon HTTP client.
type Config struct {
	// BaseURL is the daemon HTTP API base URL (e.g., "http://daemon:9080").
	BaseURL string
	// Token is the Bearer auth token (optional).
	Token string
}

// DaemonClient queries the BD daemon HTTP API for agent beads.
type DaemonClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a DaemonClient for querying the daemon's List endpoint.
func New(cfg Config) *DaemonClient {
	return &DaemonClient{
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
		token:   cfg.Token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// issueJSON mirrors the daemon's JSON response for an issue.
type issueJSON struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Type        string   `json:"type"`
	Labels      []string `json:"labels"`
	Assignee    string   `json:"assignee"`
	AgentState  string   `json:"agentState"`
}

// ListAgentBeads queries the daemon for active agent beads with the gt:agent
// label and in_progress status. It filters client-side for execution_target:k8s.
func (c *DaemonClient) ListAgentBeads(ctx context.Context) ([]AgentBead, error) {
	// Build request body matching the daemon's List RPC (ListArgs).
	// Uses labels (AND semantics) to match agent beads with k8s target.
	body := map[string]interface{}{
		"status": "in_progress",
		"labels": []string{"gt:agent", "execution_target:k8s"},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	url := c.baseURL + "/bd.v1.BeadsService/List"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	// Response is a JSON array of IssueWithCounts objects.
	var result []issueJSON
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var beads []AgentBead
	for _, issue := range result {
		if !hasLabel(issue.Labels, "execution_target:k8s") {
			continue
		}
		rig, role, name := parseAgentBeadID(issue.ID)
		if role == "" || name == "" {
			continue
		}
		beads = append(beads, AgentBead{
			ID:        issue.ID,
			Rig:       rig,
			Role:      role,
			AgentName: name,
		})
	}

	return beads, nil
}

// hasLabel checks if a label exists in the list.
func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// parseAgentBeadID parses an agent bead ID into rig, role, and name.
// Mirrors beadswatcher.parseAgentBeadID.
func parseAgentBeadID(id string) (rig, role, name string) {
	switch {
	case id == "hq-mayor":
		return "town", "mayor", "hq"
	case id == "hq-deacon":
		return "town", "deacon", "hq"
	}

	parts := strings.SplitN(id, "-", 3)
	if len(parts) == 3 {
		return parts[0], normalizeRole(parts[1]), parts[2]
	}

	return "", "", id
}

// normalizeRole converts plural role names to singular.
func normalizeRole(role string) string {
	switch role {
	case "polecats":
		return "polecat"
	case "crews":
		return "crew"
	default:
		return role
	}
}
