// Package rpcclient provides Connect-RPC clients for the Gas Town mobile API.
package rpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client wraps the Connect-RPC clients for Gas Town services.
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// NewClient creates a new RPC client.
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// Decision represents a decision from the RPC API.
type Decision struct {
	ID          string
	Question    string
	Context     string
	Options     []DecisionOption
	ChosenIndex int
	Rationale   string
	RequestedBy string
	RequestedAt string
	ResolvedBy  string
	Urgency     string
	Resolved    bool
}

// DecisionOption represents an option in a decision.
type DecisionOption struct {
	Label       string
	Description string
	Recommended bool
}

// WatchDecisions streams pending decisions from the RPC server.
// The callback is called for each new decision received.
// Returns when the context is canceled or an error occurs.
func (c *Client) WatchDecisions(ctx context.Context, callback func(Decision) error) error {
	// Polling implementation - checks for decisions every 5 seconds.
	// A full streaming implementation would use Server-Sent Events or
	// Connect-RPC's streaming support.
	seen := make(map[string]bool)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial fetch
	decisions, err := c.ListPendingDecisions(ctx)
	if err != nil {
		return fmt.Errorf("initial fetch: %w", err)
	}
	for _, d := range decisions {
		seen[d.ID] = true
		if err := callback(d); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			decisions, err := c.ListPendingDecisions(ctx)
			if err != nil {
				// Log error but continue polling
				fmt.Printf("RPC error: %v\n", err)
				continue
			}
			for _, d := range decisions {
				if !seen[d.ID] {
					seen[d.ID] = true
					if err := callback(d); err != nil {
						return err
					}
				}
			}
		}
	}
}

// ListPendingDecisions fetches pending decisions from the RPC server.
func (c *Client) ListPendingDecisions(ctx context.Context) ([]Decision, error) {
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/ListPending",
		strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Decisions []struct {
			ID          string `json:"id"`
			Question    string `json:"question"`
			Context     string `json:"context"`
			Options     []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
				Recommended bool   `json:"recommended"`
			} `json:"options"`
			ChosenIndex int    `json:"chosenIndex"`
			Rationale   string `json:"rationale"`
			RequestedBy struct {
				Name string `json:"name"`
			} `json:"requestedBy"`
			Urgency  string `json:"urgency"`
			Resolved bool   `json:"resolved"`
		} `json:"decisions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var decisions []Decision
	for _, d := range result.Decisions {
		var opts []DecisionOption
		for _, o := range d.Options {
			opts = append(opts, DecisionOption{
				Label:       o.Label,
				Description: o.Description,
				Recommended: o.Recommended,
			})
		}
		decisions = append(decisions, Decision{
			ID:          d.ID,
			Question:    d.Question,
			Context:     d.Context,
			Options:     opts,
			ChosenIndex: d.ChosenIndex,
			Rationale:   d.Rationale,
			RequestedBy: d.RequestedBy.Name,
			Urgency:     urgencyToString(d.Urgency),
			Resolved:    d.Resolved,
		})
	}

	return decisions, nil
}

func urgencyToString(u string) string {
	switch u {
	case "URGENCY_HIGH":
		return "high"
	case "URGENCY_MEDIUM":
		return "medium"
	case "URGENCY_LOW":
		return "low"
	default:
		if u != "" {
			return u
		}
		return "medium"
	}
}

func urgencyToProto(u string) string {
	switch u {
	case "high":
		return "URGENCY_HIGH"
	case "medium":
		return "URGENCY_MEDIUM"
	case "low":
		return "URGENCY_LOW"
	default:
		return "URGENCY_MEDIUM"
	}
}

// IsAvailable checks if the RPC server is available by probing the health endpoint.
// Returns true if the server responds with HTTP 200 within timeout.
func (c *Client) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// CreateDecisionRequest contains the parameters for creating a decision via RPC.
type CreateDecisionRequest struct {
	Question    string
	Context     string
	Options     []DecisionOption
	RequestedBy string
	Urgency     string
	Blockers    []string
	ParentBead  string
}

// CreateDecision creates a new decision via the RPC server.
// Returns the created decision with its assigned ID.
func (c *Client) CreateDecision(ctx context.Context, req CreateDecisionRequest) (*Decision, error) {
	// Build request body
	var options []map[string]interface{}
	for _, opt := range req.Options {
		options = append(options, map[string]interface{}{
			"label":       opt.Label,
			"description": opt.Description,
			"recommended": opt.Recommended,
		})
	}

	body := map[string]interface{}{
		"question": req.Question,
		"options":  options,
		"urgency":  urgencyToProto(req.Urgency),
	}
	if req.Context != "" {
		body["context"] = req.Context
	}
	if req.RequestedBy != "" {
		body["requestedBy"] = map[string]string{"name": req.RequestedBy}
	}
	if len(req.Blockers) > 0 {
		body["blockers"] = req.Blockers
	}
	if req.ParentBead != "" {
		body["parentBead"] = req.ParentBead
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/CreateDecision",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Decision struct {
			ID          string `json:"id"`
			Question    string `json:"question"`
			Context     string `json:"context"`
			Options     []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
				Recommended bool   `json:"recommended"`
			} `json:"options"`
			RequestedBy struct {
				Name string `json:"name"`
			} `json:"requestedBy"`
			Urgency string `json:"urgency"`
		} `json:"decision"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var opts []DecisionOption
	for _, o := range result.Decision.Options {
		opts = append(opts, DecisionOption{
			Label:       o.Label,
			Description: o.Description,
			Recommended: o.Recommended,
		})
	}

	return &Decision{
		ID:          result.Decision.ID,
		Question:    result.Decision.Question,
		Context:     result.Decision.Context,
		Options:     opts,
		RequestedBy: result.Decision.RequestedBy.Name,
		Urgency:     urgencyToString(result.Decision.Urgency),
	}, nil
}

// ResolveDecision resolves a decision via the RPC server.
// The resolvedBy parameter identifies who resolved the decision (e.g., "slack:U12345" for Slack users).
// If empty, the server defaults to "rpc-client".
func (c *Client) ResolveDecision(ctx context.Context, decisionID string, chosenIndex int, rationale, resolvedBy string) (*Decision, error) {
	body := map[string]interface{}{
		"decisionId":  decisionID,
		"chosenIndex": chosenIndex,
		"rationale":   rationale,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/Resolve",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}
	if resolvedBy != "" {
		httpReq.Header.Set("X-GT-Resolved-By", resolvedBy)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Decision struct {
			ID          string `json:"id"`
			Question    string `json:"question"`
			ChosenIndex int    `json:"chosenIndex"`
			Rationale   string `json:"rationale"`
			ResolvedBy  string `json:"resolvedBy"`
			Resolved    bool   `json:"resolved"`
		} `json:"decision"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &Decision{
		ID:          result.Decision.ID,
		Question:    result.Decision.Question,
		ChosenIndex: result.Decision.ChosenIndex,
		Rationale:   result.Decision.Rationale,
		Resolved:    result.Decision.Resolved,
	}, nil
}

// GetDecision fetches a specific decision by ID via the RPC server.
func (c *Client) GetDecision(ctx context.Context, decisionID string) (*Decision, error) {
	body := map[string]interface{}{
		"decisionId": decisionID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/GetDecision",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Decision struct {
			ID          string `json:"id"`
			Question    string `json:"question"`
			Context     string `json:"context"`
			Options     []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
				Recommended bool   `json:"recommended"`
			} `json:"options"`
			ChosenIndex int    `json:"chosenIndex"`
			Rationale   string `json:"rationale"`
			ResolvedBy  string `json:"resolvedBy"`
			RequestedBy struct {
				Name string `json:"name"`
			} `json:"requestedBy"`
			Urgency  string `json:"urgency"`
			Resolved bool   `json:"resolved"`
		} `json:"decision"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var opts []DecisionOption
	for _, o := range result.Decision.Options {
		opts = append(opts, DecisionOption{
			Label:       o.Label,
			Description: o.Description,
			Recommended: o.Recommended,
		})
	}

	return &Decision{
		ID:          result.Decision.ID,
		Question:    result.Decision.Question,
		Context:     result.Decision.Context,
		Options:     opts,
		ChosenIndex: result.Decision.ChosenIndex,
		Rationale:   result.Decision.Rationale,
		ResolvedBy:  result.Decision.ResolvedBy,
		RequestedBy: result.Decision.RequestedBy.Name,
		Urgency:     urgencyToString(result.Decision.Urgency),
		Resolved:    result.Decision.Resolved,
	}, nil
}
