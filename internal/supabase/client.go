// ABOUTME: Supabase client for Gas Town production state storage.
// ABOUTME: Uses PostgREST API directly via net/http for zero external dependencies.

package supabase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client wraps Supabase PostgREST API calls.
type Client struct {
	baseURL    string
	serviceKey string
	httpClient *http.Client
}

// NewClient creates a Supabase client from environment variables.
func NewClient() (*Client, error) {
	baseURL := os.Getenv("SUPABASE_URL")
	serviceKey := os.Getenv("SUPABASE_SERVICE_KEY")
	if baseURL == "" || serviceKey == "" {
		return nil, fmt.Errorf("SUPABASE_URL and SUPABASE_SERVICE_KEY must be set")
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		serviceKey: serviceKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// NewClientWithConfig creates a client with explicit config.
func NewClientWithConfig(baseURL, serviceKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		serviceKey: serviceKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) restURL(table string) string {
	return fmt.Sprintf("%s/rest/v1/%s", c.baseURL, table)
}

func (c *Client) doRequest(method, reqURL string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("apikey", c.serviceKey)
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")
	return c.httpClient.Do(req)
}

func (c *Client) get(table string, params url.Values, result interface{}) error {
	u := c.restURL(table)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.doRequest("GET", u, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", table, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) insert(table string, data interface{}, result interface{}) error {
	resp, err := c.doRequest("POST", c.restURL(table), data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("INSERT %s: status %d: %s", table, resp.StatusCode, string(body))
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func (c *Client) update(table string, params url.Values, data interface{}, result interface{}) error {
	u := c.restURL(table)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.doRequest("PATCH", u, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("UPDATE %s: status %d: %s", table, resp.StatusCode, string(body))
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func (c *Client) deleteRows(table string, params url.Values) error {
	u := c.restURL(table)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.doRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: status %d: %s", table, resp.StatusCode, string(body))
	}
	return nil
}

// --- Issue types ---

// Issue represents a work item stored in gt_issues.
type Issue struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Status      string          `json:"status"`
	Priority    int             `json:"priority"`
	Type        string          `json:"type,omitempty"`
	Assignee    string          `json:"assignee,omitempty"`
	ParentID    string          `json:"parent_id,omitempty"`
	Labels      []string        `json:"labels,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
}

// IssueDep represents a dependency between issues.
type IssueDep struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	DepType     string `json:"dep_type"`
}

func (c *Client) GetIssue(id string) (*Issue, error) {
	var issues []Issue
	params := url.Values{"id": {"eq." + id}}
	if err := c.get("gt_issues", params, &issues); err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("issue not found: %s", id)
	}
	return &issues[0], nil
}

func (c *Client) ListIssues(projectID string, status string) ([]Issue, error) {
	params := url.Values{"project_id": {"eq." + projectID}}
	if status != "" && status != "all" {
		params.Set("status", "eq."+status)
	}
	params.Set("order", "created_at.desc")
	var issues []Issue
	if err := c.get("gt_issues", params, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

func (c *Client) ListIssuesByAssignee(projectID, assignee string) ([]Issue, error) {
	params := url.Values{
		"project_id": {"eq." + projectID},
		"assignee":   {"eq." + assignee},
	}
	params.Set("order", "created_at.desc")
	var issues []Issue
	if err := c.get("gt_issues", params, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

func (c *Client) CreateIssue(issue *Issue) error {
	var result []Issue
	if err := c.insert("gt_issues", issue, &result); err != nil {
		return err
	}
	if len(result) > 0 {
		*issue = result[0]
	}
	return nil
}

func (c *Client) UpdateIssue(id string, updates map[string]interface{}) (*Issue, error) {
	params := url.Values{"id": {"eq." + id}}
	var result []Issue
	if err := c.update("gt_issues", params, updates, &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("issue not found: %s", id)
	}
	return &result[0], nil
}

func (c *Client) CloseIssue(id string) (*Issue, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	return c.UpdateIssue(id, map[string]interface{}{
		"status":    "closed",
		"closed_at": now,
	})
}

func (c *Client) AddDependency(issueID, dependsOnID, depType string) error {
	dep := IssueDep{IssueID: issueID, DependsOnID: dependsOnID, DepType: depType}
	return c.insert("gt_issue_deps", dep, nil)
}

func (c *Client) GetDependencies(issueID string) ([]IssueDep, error) {
	params := url.Values{"issue_id": {"eq." + issueID}}
	var deps []IssueDep
	if err := c.get("gt_issue_deps", params, &deps); err != nil {
		return nil, err
	}
	return deps, nil
}

func (c *Client) GetReadyIssues(projectID string) ([]Issue, error) {
	params := url.Values{
		"project_id": {"eq." + projectID},
		"status":     {"eq.open"},
		"assignee":   {"is.null"},
	}
	params.Set("order", "priority.asc,created_at.asc")
	var issues []Issue
	if err := c.get("gt_issues", params, &issues); err != nil {
		return nil, err
	}
	var ready []Issue
	for _, issue := range issues {
		deps, err := c.GetDependencies(issue.ID)
		if err != nil || len(deps) == 0 {
			ready = append(ready, issue)
			continue
		}
		allResolved := true
		for _, dep := range deps {
			depIssue, err := c.GetIssue(dep.DependsOnID)
			if err != nil || depIssue.Status != "closed" {
				allResolved = false
				break
			}
		}
		if allResolved {
			ready = append(ready, issue)
		}
	}
	return ready, nil
}

// --- Agent State ---

// AgentState tracks an agent's lifecycle in gt_agent_state.
type AgentState struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	AgentName         string          `json:"agent_name"`
	State             string          `json:"state"`
	HookBead          string          `json:"hook_bead,omitempty"`
	CleanupStatus     string          `json:"cleanup_status,omitempty"`
	ActiveMR          string          `json:"active_mr,omitempty"`
	NotificationLevel string          `json:"notification_level,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (c *Client) GetAgentState(id string) (*AgentState, error) {
	var agents []AgentState
	params := url.Values{"id": {"eq." + id}}
	if err := c.get("gt_agent_state", params, &agents); err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return &agents[0], nil
}

func (c *Client) UpsertAgentState(agent *AgentState) error {
	b, err := json.Marshal(agent)
	if err != nil {
		return fmt.Errorf("marshal agent: %w", err)
	}
	req, err := http.NewRequest("POST", c.restURL("gt_agent_state"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.serviceKey)
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=merge-duplicates,return=representation")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert agent_state: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// --- Messages ---

// Message represents inter-agent mail in gt_messages.
type Message struct {
	ID          string    `json:"id,omitempty"`
	ProjectID   string    `json:"project_id"`
	FromAddress string    `json:"from_address"`
	ToAddress   string    `json:"to_address"`
	Subject     string    `json:"subject,omitempty"`
	Body        string    `json:"body,omitempty"`
	Read        bool      `json:"read"`
	Priority    string    `json:"priority,omitempty"`
	ThreadID    string    `json:"thread_id,omitempty"`
	ReplyTo     string    `json:"reply_to,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (c *Client) SendMessage(msg *Message) error {
	return c.insert("gt_messages", msg, nil)
}

func (c *Client) GetInbox(projectID, address string) ([]Message, error) {
	params := url.Values{
		"project_id": {"eq." + projectID},
		"to_address": {"eq." + address},
	}
	params.Set("order", "created_at.desc")
	var msgs []Message
	if err := c.get("gt_messages", params, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (c *Client) MarkMessageRead(id string) error {
	params := url.Values{"id": {"eq." + id}}
	return c.update("gt_messages", params, map[string]interface{}{"read": true}, nil)
}

// --- Merge Requests ---

// MergeRequest represents a merge queue item in gt_merge_requests.
type MergeRequest struct {
	ID        string          `json:"id,omitempty"`
	ProjectID string          `json:"project_id"`
	Branch    string          `json:"branch"`
	Status    string          `json:"status"`
	Assignee  string          `json:"assignee,omitempty"`
	BlockedBy []string        `json:"blocked_by,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func (c *Client) ListMergeQueue(projectID string) ([]MergeRequest, error) {
	params := url.Values{
		"project_id": {"eq." + projectID},
		"status":     {"neq.merged"},
	}
	params.Set("order", "created_at.asc")
	var mrs []MergeRequest
	if err := c.get("gt_merge_requests", params, &mrs); err != nil {
		return nil, err
	}
	return mrs, nil
}

func (c *Client) SubmitMergeRequest(mr *MergeRequest) error {
	return c.insert("gt_merge_requests", mr, nil)
}

// --- Polecats ---

// Polecat represents a worker agent in gt_polecats.
type Polecat struct {
	ID        string          `json:"id,omitempty"`
	ProjectID string          `json:"project_id"`
	Name      string          `json:"name"`
	Branch    string          `json:"branch,omitempty"`
	Status    string          `json:"status"`
	HookBead  string          `json:"hook_bead,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func (c *Client) ListPolecats(projectID string) ([]Polecat, error) {
	params := url.Values{
		"project_id": {"eq." + projectID},
		"status":     {"neq.removed"},
	}
	params.Set("order", "created_at.desc")
	var polecats []Polecat
	if err := c.get("gt_polecats", params, &polecats); err != nil {
		return nil, err
	}
	return polecats, nil
}

func (c *Client) CreatePolecat(polecat *Polecat) error {
	return c.insert("gt_polecats", polecat, nil)
}

func (c *Client) UpdatePolecat(id string, updates map[string]interface{}) error {
	params := url.Values{"id": {"eq." + id}}
	return c.update("gt_polecats", params, updates, nil)
}

// --- Routes ---

// Route maps a bead prefix to a project.
type Route struct {
	Prefix    string `json:"prefix"`
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
}

func (c *Client) GetRoutes(projectID string) ([]Route, error) {
	params := url.Values{"project_id": {"eq." + projectID}}
	var routes []Route
	if err := c.get("gt_routes", params, &routes); err != nil {
		return nil, err
	}
	return routes, nil
}

// Health checks Supabase PostgREST connectivity.
func (c *Client) Health() error {
	resp, err := c.doRequest("GET", c.baseURL+"/rest/v1/", nil)
	if err != nil {
		return fmt.Errorf("supabase health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("supabase health check returned status %d", resp.StatusCode)
	}
	return nil
}
