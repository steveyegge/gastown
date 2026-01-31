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
	ID              string
	Question        string
	Context         string
	Options         []DecisionOption
	ChosenIndex     int
	Rationale       string
	RequestedBy     string
	RequestedAt     string
	ResolvedBy      string
	Urgency         string
	Resolved        bool
	PredecessorID   string // For decision chaining
	ParentBeadID    string // Parent bead ID (e.g., epic) for hierarchy
	ParentBeadTitle string // Parent bead title for channel derivation
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
	defer func() { _ = resp.Body.Close() }()

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
			ChosenIndex   int    `json:"chosenIndex"`
			Rationale     string `json:"rationale"`
			RequestedBy   struct {
				Name string `json:"name"`
			} `json:"requestedBy"`
			Urgency       string `json:"urgency"`
			Resolved      bool   `json:"resolved"`
			PredecessorID string `json:"predecessorId"`
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
			ID:            d.ID,
			Question:      d.Question,
			Context:       d.Context,
			Options:       opts,
			ChosenIndex:   d.ChosenIndex,
			Rationale:     d.Rationale,
			RequestedBy:   d.RequestedBy.Name,
			Urgency:       urgencyToString(d.Urgency),
			Resolved:      d.Resolved,
			PredecessorID: d.PredecessorID,
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
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// CreateDecisionRequest contains the parameters for creating a decision via RPC.
type CreateDecisionRequest struct {
	Question      string
	Context       string
	Options       []DecisionOption
	RequestedBy   string
	Urgency       string
	Blockers      []string
	ParentBead    string
	PredecessorID string
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
	if req.PredecessorID != "" {
		body["predecessorId"] = req.PredecessorID
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
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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

// ResolveDecisionWithCustomText resolves a decision with custom text response (the "Other" option).
// This uses chosen_index=0 to indicate a custom text response without selecting a predefined option.
// The customText parameter contains the user's response text.
func (c *Client) ResolveDecisionWithCustomText(ctx context.Context, decisionID, customText, resolvedBy string) (*Decision, error) {
	body := map[string]interface{}{
		"decisionId":  decisionID,
		"chosenIndex": 0, // 0 indicates "Other" with custom text
		"rationale":   customText,
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Decision struct {
			ID          string `json:"id"`
			Question    string `json:"question"`
			Context     string `json:"context"`
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
		Context:     result.Decision.Context,
		ChosenIndex: result.Decision.ChosenIndex,
		Rationale:   result.Decision.Rationale,
		Resolved:    result.Decision.Resolved,
	}, nil
}

// CancelDecision cancels/dismisses a decision via the RPC server.
func (c *Client) CancelDecision(ctx context.Context, decisionID, reason string) error {
	body := map[string]interface{}{
		"decisionId": decisionID,
	}
	if reason != "" {
		body["reason"] = reason
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/Cancel",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RPC error: %s", resp.Status)
	}

	return nil
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
	defer func() { _ = resp.Body.Close() }()

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
			Urgency         string `json:"urgency"`
			Resolved        bool   `json:"resolved"`
			ParentBead      string `json:"parentBead"`
			ParentBeadTitle string `json:"parentBeadTitle"`
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
		ID:              result.Decision.ID,
		Question:        result.Decision.Question,
		Context:         result.Decision.Context,
		Options:         opts,
		ChosenIndex:     result.Decision.ChosenIndex,
		Rationale:       result.Decision.Rationale,
		ResolvedBy:      result.Decision.ResolvedBy,
		RequestedBy:     result.Decision.RequestedBy.Name,
		Urgency:         urgencyToString(result.Decision.Urgency),
		Resolved:        result.Decision.Resolved,
		ParentBeadID:    result.Decision.ParentBead,
		ParentBeadTitle: result.Decision.ParentBeadTitle,
	}, nil
}

// ChainNode represents a node in a decision chain.
type ChainNode struct {
	ID            string       `json:"id"`
	Question      string       `json:"question"`
	ChosenIndex   int          `json:"chosen_index"`
	ChosenLabel   string       `json:"chosen_label,omitempty"`
	Urgency       string       `json:"urgency"`
	RequestedBy   string       `json:"requested_by"`
	RequestedAt   string       `json:"requested_at"`
	ResolvedAt    string       `json:"resolved_at,omitempty"`
	PredecessorID string       `json:"predecessor_id,omitempty"`
	Children      []*ChainNode `json:"children,omitempty"`
	IsTarget      bool         `json:"is_target,omitempty"`
}

// GetDecisionChain fetches the ancestry chain for a decision via the RPC server.
// Returns the chain from root to the specified decision.
func (c *Client) GetDecisionChain(ctx context.Context, decisionID string) ([]*ChainNode, error) {
	body := map[string]interface{}{
		"decisionId": decisionID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/GetChain",
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Chain []struct {
			ID            string `json:"id"`
			Question      string `json:"question"`
			ChosenIndex   int    `json:"chosenIndex"`
			ChosenLabel   string `json:"chosenLabel"`
			Urgency       string `json:"urgency"`
			RequestedBy   string `json:"requestedBy"`
			RequestedAt   string `json:"requestedAt"`
			ResolvedAt    string `json:"resolvedAt"`
			PredecessorID string `json:"predecessorId"`
			IsTarget      bool   `json:"isTarget"`
		} `json:"chain"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var chain []*ChainNode
	for _, node := range result.Chain {
		chain = append(chain, &ChainNode{
			ID:            node.ID,
			Question:      node.Question,
			ChosenIndex:   node.ChosenIndex,
			ChosenLabel:   node.ChosenLabel,
			Urgency:       urgencyToString(node.Urgency),
			RequestedBy:   node.RequestedBy,
			RequestedAt:   node.RequestedAt,
			ResolvedAt:    node.ResolvedAt,
			PredecessorID: node.PredecessorID,
			IsTarget:      node.IsTarget,
		})
	}

	return chain, nil
}

// ValidateDecisionContext validates the JSON context for a decision.
// Returns validation errors if the context is invalid.
func (c *Client) ValidateDecisionContext(ctx context.Context, context string, predecessorID string) (bool, []string, error) {
	body := map[string]interface{}{
		"context": context,
	}
	if predecessorID != "" {
		body["predecessorId"] = predecessorID
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return false, nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.DecisionService/ValidateContext",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return false, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	// Parse response
	var result struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.Valid, result.Errors, nil
}

// Convoy represents a convoy from the RPC API.
type Convoy struct {
	ID             string
	Title          string
	Status         string
	Owner          string
	Notify         string
	Molecule       string
	Owned          bool
	MergeStrategy  string
	CreatedAt      string
	ClosedAt       string
	TrackedCount   int
	CompletedCount int
	Progress       string
}

// TrackedIssue represents an issue tracked by a convoy.
type TrackedIssue struct {
	ID        string
	Title     string
	Status    string
	IssueType string
	Assignee  string
	Worker    string
	WorkerAge string
}

// ListConvoys fetches convoys from the RPC server.
// status: "open", "closed", "all"
func (c *Client) ListConvoys(ctx context.Context, status string, tree bool) ([]Convoy, error) {
	body := map[string]interface{}{}
	switch status {
	case "open":
		body["status"] = "CONVOY_STATUS_FILTER_OPEN"
	case "closed":
		body["status"] = "CONVOY_STATUS_FILTER_CLOSED"
	case "all":
		body["status"] = "CONVOY_STATUS_FILTER_ALL"
	}
	if tree {
		body["tree"] = true
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ConvoyService/ListConvoys",
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Convoys []struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Status         string `json:"status"`
			Owner          string `json:"owner"`
			Notify         string `json:"notify"`
			Molecule       string `json:"molecule"`
			Owned          bool   `json:"owned"`
			MergeStrategy  string `json:"mergeStrategy"`
			CreatedAt      string `json:"createdAt"`
			ClosedAt       string `json:"closedAt"`
			TrackedCount   int    `json:"trackedCount"`
			CompletedCount int    `json:"completedCount"`
			Progress       string `json:"progress"`
		} `json:"convoys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var convoys []Convoy
	for _, c := range result.Convoys {
		convoys = append(convoys, Convoy{
			ID:             c.ID,
			Title:          c.Title,
			Status:         c.Status,
			Owner:          c.Owner,
			Notify:         c.Notify,
			Molecule:       c.Molecule,
			Owned:          c.Owned,
			MergeStrategy:  c.MergeStrategy,
			CreatedAt:      c.CreatedAt,
			ClosedAt:       c.ClosedAt,
			TrackedCount:   c.TrackedCount,
			CompletedCount: c.CompletedCount,
			Progress:       c.Progress,
		})
	}

	return convoys, nil
}

// GetConvoyStatus fetches detailed status for a convoy.
func (c *Client) GetConvoyStatus(ctx context.Context, convoyID string) (*Convoy, []TrackedIssue, error) {
	body := map[string]interface{}{
		"convoyId": convoyID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ConvoyService/GetConvoyStatus",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Convoy struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Status         string `json:"status"`
			Owner          string `json:"owner"`
			Notify         string `json:"notify"`
			Molecule       string `json:"molecule"`
			Owned          bool   `json:"owned"`
			MergeStrategy  string `json:"mergeStrategy"`
			CreatedAt      string `json:"createdAt"`
			ClosedAt       string `json:"closedAt"`
			TrackedCount   int    `json:"trackedCount"`
			CompletedCount int    `json:"completedCount"`
			Progress       string `json:"progress"`
		} `json:"convoy"`
		Tracked []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Status    string `json:"status"`
			IssueType string `json:"issueType"`
			Assignee  string `json:"assignee"`
			Worker    string `json:"worker"`
			WorkerAge string `json:"workerAge"`
		} `json:"tracked"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decoding response: %w", err)
	}

	convoy := &Convoy{
		ID:             result.Convoy.ID,
		Title:          result.Convoy.Title,
		Status:         result.Convoy.Status,
		Owner:          result.Convoy.Owner,
		Notify:         result.Convoy.Notify,
		Molecule:       result.Convoy.Molecule,
		Owned:          result.Convoy.Owned,
		MergeStrategy:  result.Convoy.MergeStrategy,
		CreatedAt:      result.Convoy.CreatedAt,
		ClosedAt:       result.Convoy.ClosedAt,
		TrackedCount:   result.Convoy.TrackedCount,
		CompletedCount: result.Convoy.CompletedCount,
		Progress:       result.Convoy.Progress,
	}

	var tracked []TrackedIssue
	for _, t := range result.Tracked {
		tracked = append(tracked, TrackedIssue{
			ID:        t.ID,
			Title:     t.Title,
			Status:    t.Status,
			IssueType: t.IssueType,
			Assignee:  t.Assignee,
			Worker:    t.Worker,
			WorkerAge: t.WorkerAge,
		})
	}

	return convoy, tracked, nil
}

// CreateConvoyRequest contains the parameters for creating a convoy via RPC.
type CreateConvoyRequest struct {
	Name          string
	IssueIDs      []string
	Owner         string
	Notify        string
	Molecule      string
	Owned         bool
	MergeStrategy string
}

// CreateConvoy creates a new convoy via the RPC server.
func (c *Client) CreateConvoy(ctx context.Context, req CreateConvoyRequest) (*Convoy, int, error) {
	body := map[string]interface{}{
		"name":     req.Name,
		"issueIds": req.IssueIDs,
	}
	if req.Owner != "" {
		body["owner"] = req.Owner
	}
	if req.Notify != "" {
		body["notify"] = req.Notify
	}
	if req.Molecule != "" {
		body["molecule"] = req.Molecule
	}
	if req.Owned {
		body["owned"] = true
	}
	if req.MergeStrategy != "" {
		body["mergeStrategy"] = req.MergeStrategy
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ConvoyService/CreateConvoy",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Convoy struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Status         string `json:"status"`
			Owner          string `json:"owner"`
			Notify         string `json:"notify"`
			Molecule       string `json:"molecule"`
			Owned          bool   `json:"owned"`
			MergeStrategy  string `json:"mergeStrategy"`
			TrackedCount   int    `json:"trackedCount"`
			CompletedCount int    `json:"completedCount"`
			Progress       string `json:"progress"`
		} `json:"convoy"`
		TrackedCount int `json:"trackedCount"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	convoy := &Convoy{
		ID:             result.Convoy.ID,
		Title:          result.Convoy.Title,
		Status:         result.Convoy.Status,
		Owner:          result.Convoy.Owner,
		Notify:         result.Convoy.Notify,
		Molecule:       result.Convoy.Molecule,
		Owned:          result.Convoy.Owned,
		MergeStrategy:  result.Convoy.MergeStrategy,
		TrackedCount:   result.Convoy.TrackedCount,
		CompletedCount: result.Convoy.CompletedCount,
		Progress:       result.Convoy.Progress,
	}

	return convoy, result.TrackedCount, nil
}

// AddToConvoy adds issues to an existing convoy.
func (c *Client) AddToConvoy(ctx context.Context, convoyID string, issueIDs []string) (*Convoy, int, bool, error) {
	body := map[string]interface{}{
		"convoyId": convoyID,
		"issueIds": issueIDs,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, false, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ConvoyService/AddToConvoy",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, 0, false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, false, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Convoy struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"convoy"`
		AddedCount int  `json:"addedCount"`
		Reopened   bool `json:"reopened"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, false, fmt.Errorf("decoding response: %w", err)
	}

	convoy := &Convoy{
		ID:     result.Convoy.ID,
		Title:  result.Convoy.Title,
		Status: result.Convoy.Status,
	}

	return convoy, result.AddedCount, result.Reopened, nil
}

// CloseConvoy closes a convoy via the RPC server.
func (c *Client) CloseConvoy(ctx context.Context, convoyID, reason, notify string) (*Convoy, error) {
	body := map[string]interface{}{
		"convoyId": convoyID,
	}
	if reason != "" {
		body["reason"] = reason
	}
	if notify != "" {
		body["notify"] = notify
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ConvoyService/CloseConvoy",
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Convoy struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"convoy"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	convoy := &Convoy{
		ID:     result.Convoy.ID,
		Title:  result.Convoy.Title,
		Status: result.Convoy.Status,
	}

	return convoy, nil
}

// ActivityEvent represents an event from the activity feed.
type ActivityEvent struct {
	Timestamp  string
	Source     string
	Type       string
	Actor      string
	Payload    map[string]interface{}
	Visibility string
	Summary    string // For curated feed events
	Count      int    // For aggregated events
}

// EventFilter specifies criteria for filtering events.
type EventFilter struct {
	Types      []string // Filter by event types (empty = all)
	Actor      string   // Filter by actor (empty = all)
	Rig        string   // Filter by rig (empty = all)
	Visibility string   // Filter by visibility ("audit", "feed", "both", or empty for all)
	After      string   // Only events after this timestamp (RFC3339)
	Before     string   // Only events before this timestamp (RFC3339)
}

// ListEventsRequest contains the parameters for listing events via RPC.
type ListEventsRequest struct {
	Filter  *EventFilter
	Limit   int  // Maximum events (default: 100, max: 1000)
	Curated bool // Read from curated feed (true) or raw events (false)
}

// ListEvents fetches events from the activity feed.
func (c *Client) ListEvents(ctx context.Context, req ListEventsRequest) ([]ActivityEvent, int, error) {
	body := map[string]interface{}{
		"curated": req.Curated,
	}
	if req.Limit > 0 {
		body["limit"] = req.Limit
	}
	if req.Filter != nil {
		filter := map[string]interface{}{}
		if len(req.Filter.Types) > 0 {
			filter["types"] = req.Filter.Types
		}
		if req.Filter.Actor != "" {
			filter["actor"] = req.Filter.Actor
		}
		if req.Filter.Rig != "" {
			filter["rig"] = req.Filter.Rig
		}
		if req.Filter.Visibility != "" {
			filter["visibility"] = visibilityToProto(req.Filter.Visibility)
		}
		if req.Filter.After != "" {
			filter["after"] = req.Filter.After
		}
		if req.Filter.Before != "" {
			filter["before"] = req.Filter.Before
		}
		if len(filter) > 0 {
			body["filter"] = filter
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ActivityService/ListEvents",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Events []struct {
			Timestamp  string                 `json:"timestamp"`
			Source     string                 `json:"source"`
			Type       string                 `json:"type"`
			Actor      string                 `json:"actor"`
			Payload    map[string]interface{} `json:"payload"`
			Visibility string                 `json:"visibility"`
			Summary    string                 `json:"summary"`
			Count      int                    `json:"count"`
		} `json:"events"`
		TotalCount int `json:"totalCount"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	var events []ActivityEvent
	for _, e := range result.Events {
		events = append(events, ActivityEvent{
			Timestamp:  e.Timestamp,
			Source:     e.Source,
			Type:       e.Type,
			Actor:      e.Actor,
			Payload:    e.Payload,
			Visibility: visibilityToString(e.Visibility),
			Summary:    e.Summary,
			Count:      e.Count,
		})
	}

	return events, result.TotalCount, nil
}

// WatchEvents streams events from the activity feed.
// The callback is called for each new event received.
// Returns when the context is canceled or an error occurs.
func (c *Client) WatchEvents(ctx context.Context, filter *EventFilter, curated bool, callback func(ActivityEvent) error) error {
	// Polling implementation - checks for events every second.
	// Track seen events by timestamp to avoid duplicates
	lastTimestamp := ""
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Build filter with "after" to only get new events
			reqFilter := filter
			if lastTimestamp != "" {
				if reqFilter == nil {
					reqFilter = &EventFilter{}
				} else {
					// Copy filter to avoid mutating the original
					filterCopy := *reqFilter
					reqFilter = &filterCopy
				}
				reqFilter.After = lastTimestamp
			}

			events, _, err := c.ListEvents(ctx, ListEventsRequest{
				Filter:  reqFilter,
				Limit:   100,
				Curated: curated,
			})
			if err != nil {
				// Log error but continue polling
				fmt.Printf("RPC error: %v\n", err)
				continue
			}

			// Events are returned newest first, so process in reverse
			for i := len(events) - 1; i >= 0; i-- {
				e := events[i]
				if e.Timestamp > lastTimestamp {
					lastTimestamp = e.Timestamp
					if err := callback(e); err != nil {
						return err
					}
				}
			}
		}
	}
}

// EmitEventRequest contains the parameters for emitting an event via RPC.
type EmitEventRequest struct {
	Type       string
	Actor      string
	Payload    map[string]interface{}
	Visibility string // "audit", "feed", or "both" (default: "feed")
}

// EmitEvent writes a new event to the activity log via RPC.
func (c *Client) EmitEvent(ctx context.Context, req EmitEventRequest) (string, error) {
	body := map[string]interface{}{
		"type":  req.Type,
		"actor": req.Actor,
	}
	if len(req.Payload) > 0 {
		body["payload"] = req.Payload
	}
	if req.Visibility != "" {
		body["visibility"] = visibilityToProto(req.Visibility)
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.ActivityService/EmitEvent",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Timestamp string `json:"timestamp"`
		Success   bool   `json:"success"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("emit event failed")
	}

	return result.Timestamp, nil
}

func visibilityToProto(v string) string {
	switch v {
	case "audit":
		return "VISIBILITY_AUDIT"
	case "feed":
		return "VISIBILITY_FEED"
	case "both":
		return "VISIBILITY_BOTH"
	default:
		return "VISIBILITY_FEED"
	}
}

func visibilityToString(v string) string {
	switch v {
	case "VISIBILITY_AUDIT":
		return "audit"
	case "VISIBILITY_FEED":
		return "feed"
	case "VISIBILITY_BOTH":
		return "both"
	default:
		if v != "" {
			return v
		}
		return "feed"
	}
}

// MailMessage represents a mail message from the RPC API.
type MailMessage struct {
	ID        string
	From      string
	To        string
	Subject   string
	Body      string
	Timestamp time.Time
	Read      bool
	Priority  string
	Type      string
	ThreadID  string
	ReplyTo   string
	Pinned    bool
	CC        []string
}

// ListInboxRequest contains the parameters for listing inbox messages.
type ListInboxRequest struct {
	Address    string // Recipient address (empty = overseer)
	UnreadOnly bool
	Limit      int
}

// ListInbox fetches messages from an inbox via RPC.
func (c *Client) ListInbox(ctx context.Context, req ListInboxRequest) ([]MailMessage, int, int, error) {
	body := map[string]interface{}{}
	if req.Address != "" {
		body["address"] = map[string]string{"name": req.Address}
	}
	if req.UnreadOnly {
		body["unreadOnly"] = true
	}
	if req.Limit > 0 {
		body["limit"] = req.Limit
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.MailService/ListInbox",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, 0, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, 0, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Messages []struct {
			ID        string `json:"id"`
			From      struct{ Name string } `json:"from"`
			To        struct{ Name string } `json:"to"`
			Subject   string `json:"subject"`
			Body      string `json:"body"`
			Timestamp string `json:"timestamp"`
			Read      bool   `json:"read"`
			Priority  string `json:"priority"`
			Type      string `json:"type"`
			ThreadID  string `json:"threadId"`
			ReplyTo   string `json:"replyTo"`
			Pinned    bool   `json:"pinned"`
			CC        []struct{ Name string } `json:"cc"`
		} `json:"messages"`
		Total  int `json:"total"`
		Unread int `json:"unread"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, 0, fmt.Errorf("decoding response: %w", err)
	}

	var messages []MailMessage
	for _, m := range result.Messages {
		ts, _ := time.Parse(time.RFC3339, m.Timestamp)
		msg := MailMessage{
			ID:        m.ID,
			From:      m.From.Name,
			To:        m.To.Name,
			Subject:   m.Subject,
			Body:      m.Body,
			Timestamp: ts,
			Read:      m.Read,
			Priority:  priorityToString(m.Priority),
			Type:      messageTypeToString(m.Type),
			ThreadID:  m.ThreadID,
			ReplyTo:   m.ReplyTo,
			Pinned:    m.Pinned,
		}
		for _, cc := range m.CC {
			msg.CC = append(msg.CC, cc.Name)
		}
		messages = append(messages, msg)
	}

	return messages, result.Total, result.Unread, nil
}

// ReadMailMessage fetches a specific message by ID via RPC.
func (c *Client) ReadMailMessage(ctx context.Context, messageID string) (*MailMessage, error) {
	body := map[string]interface{}{
		"messageId": messageID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.MailService/ReadMessage",
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Message struct {
			ID        string `json:"id"`
			From      struct{ Name string } `json:"from"`
			To        struct{ Name string } `json:"to"`
			Subject   string `json:"subject"`
			Body      string `json:"body"`
			Timestamp string `json:"timestamp"`
			Read      bool   `json:"read"`
			Priority  string `json:"priority"`
			Type      string `json:"type"`
			ThreadID  string `json:"threadId"`
			ReplyTo   string `json:"replyTo"`
			Pinned    bool   `json:"pinned"`
			CC        []struct{ Name string } `json:"cc"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	m := result.Message
	ts, _ := time.Parse(time.RFC3339, m.Timestamp)
	msg := &MailMessage{
		ID:        m.ID,
		From:      m.From.Name,
		To:        m.To.Name,
		Subject:   m.Subject,
		Body:      m.Body,
		Timestamp: ts,
		Read:      m.Read,
		Priority:  priorityToString(m.Priority),
		Type:      messageTypeToString(m.Type),
		ThreadID:  m.ThreadID,
		ReplyTo:   m.ReplyTo,
		Pinned:    m.Pinned,
	}
	for _, cc := range m.CC {
		msg.CC = append(msg.CC, cc.Name)
	}

	return msg, nil
}

// SendMailRequest contains the parameters for sending a mail message via RPC.
type SendMailRequest struct {
	To       string
	Subject  string
	Body     string
	Priority string
	Type     string
	ReplyTo  string
	CC       []string
}

// SendMail sends a new mail message via RPC.
func (c *Client) SendMail(ctx context.Context, req SendMailRequest) (string, error) {
	body := map[string]interface{}{
		"to":      map[string]string{"name": req.To},
		"subject": req.Subject,
		"body":    req.Body,
	}
	if req.Priority != "" {
		body["priority"] = priorityToProto(req.Priority)
	}
	if req.Type != "" {
		body["type"] = messageTypeToProto(req.Type)
	}
	if req.ReplyTo != "" {
		body["replyTo"] = req.ReplyTo
	}
	if len(req.CC) > 0 {
		var ccAddrs []map[string]string
		for _, cc := range req.CC {
			ccAddrs = append(ccAddrs, map[string]string{"name": cc})
		}
		body["cc"] = ccAddrs
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.MailService/SendMessage",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		MessageID string `json:"messageId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return result.MessageID, nil
}

// MarkMailRead marks a mail message as read via RPC.
func (c *Client) MarkMailRead(ctx context.Context, messageID string) error {
	body := map[string]interface{}{
		"messageId": messageID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.MailService/MarkRead",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RPC error: %s", resp.Status)
	}

	return nil
}

// DeleteMail deletes/archives a mail message via RPC.
func (c *Client) DeleteMail(ctx context.Context, messageID string) error {
	body := map[string]interface{}{
		"messageId": messageID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.MailService/DeleteMessage",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RPC error: %s", resp.Status)
	}

	return nil
}

func priorityToString(p string) string {
	switch p {
	case "PRIORITY_URGENT":
		return "urgent"
	case "PRIORITY_HIGH":
		return "high"
	case "PRIORITY_NORMAL":
		return "normal"
	case "PRIORITY_LOW":
		return "low"
	default:
		if p != "" {
			return p
		}
		return "normal"
	}
}

func priorityToProto(p string) string {
	switch p {
	case "urgent":
		return "PRIORITY_URGENT"
	case "high":
		return "PRIORITY_HIGH"
	case "normal":
		return "PRIORITY_NORMAL"
	case "low":
		return "PRIORITY_LOW"
	default:
		return "PRIORITY_NORMAL"
	}
}

func messageTypeToString(t string) string {
	switch t {
	case "MESSAGE_TYPE_TASK":
		return "task"
	case "MESSAGE_TYPE_SCAVENGE":
		return "scavenge"
	case "MESSAGE_TYPE_NOTIFICATION":
		return "notification"
	case "MESSAGE_TYPE_REPLY":
		return "reply"
	default:
		if t != "" {
			return t
		}
		return "notification"
	}
}

func messageTypeToProto(t string) string {
	switch t {
	case "task":
		return "MESSAGE_TYPE_TASK"
	case "scavenge":
		return "MESSAGE_TYPE_SCAVENGE"
	case "notification":
		return "MESSAGE_TYPE_NOTIFICATION"
	case "reply":
		return "MESSAGE_TYPE_REPLY"
	default:
		return "MESSAGE_TYPE_NOTIFICATION"
	}
}

// PeekSession captures the last N lines from a tmux session's pane via RPC.
func (c *Client) PeekSession(ctx context.Context, session string, lines int, all bool) (string, []string, bool, error) {
	body := map[string]interface{}{
		"session": session,
	}
	if lines > 0 {
		body["lines"] = lines
	}
	if all {
		body["all"] = true
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", nil, false, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.TerminalService/PeekSession",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", nil, false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", nil, false, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Output string   `json:"output"`
		Lines  []string `json:"lines"`
		Exists bool     `json:"exists"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, false, fmt.Errorf("decoding response: %w", err)
	}

	return result.Output, result.Lines, result.Exists, nil
}

// ListTmuxSessions returns all active tmux sessions via RPC.
func (c *Client) ListTmuxSessions(ctx context.Context, prefix string) ([]string, error) {
	body := map[string]interface{}{}
	if prefix != "" {
		body["prefix"] = prefix
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.TerminalService/ListSessions",
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Sessions []string `json:"sessions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.Sessions, nil
}

// HasTmuxSession checks if a specific tmux session exists via RPC.
func (c *Client) HasTmuxSession(ctx context.Context, session string) (bool, error) {
	body := map[string]interface{}{
		"session": session,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.TerminalService/HasSession",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Exists bool `json:"exists"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decoding response: %w", err)
	}

	return result.Exists, nil
}

// TerminalUpdate represents a streaming update of terminal content.
type TerminalUpdate struct {
	Output    string
	Lines     []string
	Exists    bool
	Timestamp string
}

// WatchSession streams terminal output updates via RPC.
// The callback is called for each terminal update received.
// Returns when the context is canceled, the session dies, or an error occurs.
func (c *Client) WatchSession(ctx context.Context, session string, lines, intervalMs int, callback func(TerminalUpdate) error) error {
	// Polling implementation
	if lines <= 0 {
		lines = 50
	}
	if intervalMs < 100 {
		intervalMs = 1000
	}

	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			output, lineSlice, exists, err := c.PeekSession(ctx, session, lines, false)
			if err != nil {
				// Log error but continue polling
				continue
			}

			update := TerminalUpdate{
				Output:    output,
				Lines:     lineSlice,
				Exists:    exists,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}

			if err := callback(update); err != nil {
				return err
			}

			// Stop if session no longer exists
			if !exists {
				return nil
			}
		}
	}
}
