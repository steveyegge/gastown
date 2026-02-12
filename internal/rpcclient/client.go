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
	townName   string // Town name for bead ID generation (set via WithTownName)
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

// WithTownName sets the town name used for bead ID generation.
func WithTownName(name string) Option {
	return func(c *Client) {
		c.townName = name
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
	PredecessorID   string   // For decision chaining
	ParentBeadID    string   // Parent bead ID (e.g., epic) for hierarchy
	ParentBeadTitle string   // Parent bead title for channel derivation
	Blockers        []string // Work IDs blocked by this decision (gt-subr1i.4)
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
			Urgency       string   `json:"urgency"`
			Resolved      bool     `json:"resolved"`
			PredecessorID string   `json:"predecessorId"`
			Blockers      []string `json:"blockers"` // (gt-subr1i.4)
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
			Blockers:      d.Blockers, // (gt-subr1i.4)
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
			Urgency  string   `json:"urgency"`
			Blockers []string `json:"blockers"` // (gt-subr1i.4)
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
		Blockers:    result.Decision.Blockers, // (gt-subr1i.4)
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
			Urgency         string   `json:"urgency"`
			Resolved        bool     `json:"resolved"`
			ParentBead      string   `json:"parentBead"`
			ParentBeadTitle string   `json:"parentBeadTitle"`
			Blockers        []string `json:"blockers"` // (gt-subr1i.4)
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
		Blockers:        result.Decision.Blockers, // (gt-subr1i.4)
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

// ============================================================================
// SlingService Client Methods
// ============================================================================

// SlingRequest contains the parameters for slinging work via RPC.
type SlingRequest struct {
	BeadID        string
	Target        string
	Args          string
	Subject       string
	Message       string
	Create        bool
	Force         bool
	NoConvoy      bool
	Convoy        string
	NoMerge       bool
	MergeStrategy string
	Owned         bool
	Account       string
	Agent         string
}

// SlingResponse contains the result of slinging work.
type SlingResponse struct {
	BeadID         string
	TargetAgent    string
	ConvoyID       string
	PolecatSpawned bool
	PolecatName    string
	BeadTitle      string
	ConvoyCreated  bool
}

// Sling assigns a bead to a target agent via RPC.
func (c *Client) Sling(ctx context.Context, req SlingRequest) (*SlingResponse, error) {
	body := map[string]interface{}{
		"beadId": req.BeadID,
	}
	if req.Target != "" {
		body["target"] = req.Target
	}
	if req.Args != "" {
		body["args"] = req.Args
	}
	if req.Subject != "" {
		body["subject"] = req.Subject
	}
	if req.Message != "" {
		body["message"] = req.Message
	}
	if req.Create {
		body["create"] = true
	}
	if req.Force {
		body["force"] = true
	}
	if req.NoConvoy {
		body["noConvoy"] = true
	}
	if req.Convoy != "" {
		body["convoy"] = req.Convoy
	}
	if req.NoMerge {
		body["noMerge"] = true
	}
	if req.MergeStrategy != "" {
		body["mergeStrategy"] = mergeStrategyToProto(req.MergeStrategy)
	}
	if req.Owned {
		body["owned"] = true
	}
	if req.Account != "" {
		body["account"] = req.Account
	}
	if req.Agent != "" {
		body["agent"] = req.Agent
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.SlingService/Sling",
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
		BeadID         string `json:"beadId"`
		TargetAgent    string `json:"targetAgent"`
		ConvoyID       string `json:"convoyId"`
		PolecatSpawned bool   `json:"polecatSpawned"`
		PolecatName    string `json:"polecatName"`
		BeadTitle      string `json:"beadTitle"`
		ConvoyCreated  bool   `json:"convoyCreated"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &SlingResponse{
		BeadID:         result.BeadID,
		TargetAgent:    result.TargetAgent,
		ConvoyID:       result.ConvoyID,
		PolecatSpawned: result.PolecatSpawned,
		PolecatName:    result.PolecatName,
		BeadTitle:      result.BeadTitle,
		ConvoyCreated:  result.ConvoyCreated,
	}, nil
}

// SlingFormulaRequest contains the parameters for slinging a formula via RPC.
type SlingFormulaRequest struct {
	Formula string
	Target  string
	OnBead  string
	Vars    map[string]string
	Args    string
	Subject string
	Message string
	Create  bool
	Force   bool
	Account string
	Agent   string
}

// SlingFormulaResponse contains the result of slinging a formula.
type SlingFormulaResponse struct {
	WispID         string
	TargetAgent    string
	BeadID         string
	ConvoyID       string
	PolecatSpawned bool
	PolecatName    string
}

// SlingFormula instantiates and slings a formula via RPC.
func (c *Client) SlingFormula(ctx context.Context, req SlingFormulaRequest) (*SlingFormulaResponse, error) {
	body := map[string]interface{}{
		"formula": req.Formula,
	}
	if req.Target != "" {
		body["target"] = req.Target
	}
	if req.OnBead != "" {
		body["onBead"] = req.OnBead
	}
	if len(req.Vars) > 0 {
		body["vars"] = req.Vars
	}
	if req.Args != "" {
		body["args"] = req.Args
	}
	if req.Subject != "" {
		body["subject"] = req.Subject
	}
	if req.Message != "" {
		body["message"] = req.Message
	}
	if req.Create {
		body["create"] = true
	}
	if req.Force {
		body["force"] = true
	}
	if req.Account != "" {
		body["account"] = req.Account
	}
	if req.Agent != "" {
		body["agent"] = req.Agent
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.SlingService/SlingFormula",
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
		WispID         string `json:"wispId"`
		TargetAgent    string `json:"targetAgent"`
		BeadID         string `json:"beadId"`
		ConvoyID       string `json:"convoyId"`
		PolecatSpawned bool   `json:"polecatSpawned"`
		PolecatName    string `json:"polecatName"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &SlingFormulaResponse{
		WispID:         result.WispID,
		TargetAgent:    result.TargetAgent,
		BeadID:         result.BeadID,
		ConvoyID:       result.ConvoyID,
		PolecatSpawned: result.PolecatSpawned,
		PolecatName:    result.PolecatName,
	}, nil
}

// Unsling removes work from an agent's hook via RPC.
func (c *Client) Unsling(ctx context.Context, beadID string, force bool) (string, string, bool, error) {
	body := map[string]interface{}{
		"beadId": beadID,
	}
	if force {
		body["force"] = true
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", "", false, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.SlingService/Unsling",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", "", false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		BeadID        string `json:"beadId"`
		PreviousAgent string `json:"previousAgent"`
		WasIncomplete bool   `json:"wasIncomplete"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", false, fmt.Errorf("decoding response: %w", err)
	}

	return result.BeadID, result.PreviousAgent, result.WasIncomplete, nil
}

// HookedBead represents a bead hooked to an agent.
type HookedBead struct {
	ID               string
	Title            string
	BeadType         string
	Priority         string
	HookedAt         string
	AttachedMolecule string
	ConvoyID         string
}

// GetWorkload returns all hooked beads for an agent via RPC.
func (c *Client) GetWorkload(ctx context.Context, agent string) ([]HookedBead, int, error) {
	body := map[string]interface{}{
		"agent": agent,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.SlingService/GetWorkload",
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
		Beads []struct {
			ID               string `json:"id"`
			Title            string `json:"title"`
			BeadType         string `json:"beadType"`
			Priority         string `json:"priority"`
			HookedAt         string `json:"hookedAt"`
			AttachedMolecule string `json:"attachedMolecule"`
			ConvoyID         string `json:"convoyId"`
		} `json:"beads"`
		Total int `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	var beads []HookedBead
	for _, b := range result.Beads {
		beads = append(beads, HookedBead{
			ID:               b.ID,
			Title:            b.Title,
			BeadType:         b.BeadType,
			Priority:         b.Priority,
			HookedAt:         b.HookedAt,
			AttachedMolecule: b.AttachedMolecule,
			ConvoyID:         b.ConvoyID,
		})
	}

	return beads, result.Total, nil
}

func mergeStrategyToProto(s string) string {
	switch s {
	case "direct":
		return "MERGE_STRATEGY_DIRECT"
	case "mr":
		return "MERGE_STRATEGY_MR"
	case "local":
		return "MERGE_STRATEGY_LOCAL"
	default:
		return "MERGE_STRATEGY_UNSPECIFIED"
	}
}

// ============================================================================
// AgentService Client Methods — backed by bd.v1.BeadsService HTTP API
// ============================================================================

// Agent represents an agent (crew worker or polecat) from the RPC API.
type Agent struct {
	Address      string
	Name         string
	Rig          string
	Type         string
	State        string
	Session      string
	WorkDir      string
	Branch       string
	HookedBead   string
	HookedTitle  string
	UnreadMail   int
	StartedAt    string
	LastActivity string
	GitStatus    string
	ConvoyID     string
}

// postBeads sends a POST to /bd.v1.BeadsService/{method} and decodes the
// JSON response into dst.  The bd daemon uses Authorization: Bearer <token>
// for auth.
func (c *Client) postBeads(ctx context.Context, method string, body interface{}, dst interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/bd.v1.BeadsService/"+method,
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Try to extract error message from body.
		var errBody struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&errBody) == nil && errBody.Error != "" {
			return fmt.Errorf("beads API %s: %s", method, errBody.Error)
		}
		return fmt.Errorf("beads API %s: %s", method, resp.Status)
	}

	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return fmt.Errorf("decoding %s response: %w", method, err)
		}
	}
	return nil
}

// crewBeadID builds the deterministic bead ID for a crew worker.
// Format: hq-<town>-<rig>-crew-<name> (or hq-<rig>-crew-<name> when town is empty).
func crewBeadID(town, rig, name string) string {
	if town == "" {
		return fmt.Sprintf("hq-%s-crew-%s", rig, name)
	}
	return fmt.Sprintf("hq-%s-%s-crew-%s", town, rig, name)
}

// agentBeadID builds a bead ID from an agent address (rig/role/name).
func (c *Client) agentBeadID(agentAddr string) string {
	parts := strings.Split(agentAddr, "/")
	if len(parts) == 3 {
		if c.townName != "" {
			return fmt.Sprintf("hq-%s-%s-%s-%s", c.townName, parts[0], parts[1], parts[2])
		}
		return fmt.Sprintf("hq-%s-%s-%s", parts[0], parts[1], parts[2])
	}
	return agentAddr
}

// beadIssue is the JSON shape returned by bd.v1.BeadsService for issue fields.
type beadIssue struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	IssueType    string `json:"issue_type"`
	RoleType     string `json:"role_type"`
	Rig          string `json:"rig"`
	AgentState   string `json:"agent_state"`
	HookBead     string `json:"hook_bead"`
	PodName      string `json:"pod_name"`
	PodIP        string `json:"pod_ip"`
	PodStatus    string `json:"pod_status"`
	Branch       string `json:"branch"`
	StartedAt    string `json:"started_at"`
	LastActivity string `json:"last_activity"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// agentFromBead converts a beadIssue into an Agent, extracting the name
// from the bead ID suffix.
func agentFromBead(b beadIssue) Agent {
	name := ""
	roleType := b.RoleType
	if roleType == "" {
		roleType = "crew"
	}
	sep := roleType + "-"
	if idx := strings.LastIndex(b.ID, sep); idx >= 0 {
		name = b.ID[idx+len(sep):]
	}

	return Agent{
		Address:      fmt.Sprintf("%s/%s/%s", b.Rig, roleType, name),
		Name:         name,
		Rig:          b.Rig,
		Type:         roleType,
		State:        b.AgentState,
		HookedBead:   b.HookBead,
		StartedAt:    b.StartedAt,
		LastActivity: b.LastActivity,
	}
}

// ListAgents fetches agent beads from the daemon's beads API.
// It queries beads with label "gt:agent" and optional "role:<type>" and "rig:<rig>".
func (c *Client) ListAgents(ctx context.Context, rig string, agentType string, includeStopped, includeGlobal bool) ([]Agent, int, int, error) {
	labels := []string{"gt:agent"}
	if agentType != "" {
		labels = append(labels, "role:"+agentType)
	}
	if rig != "" {
		labels = append(labels, "rig:"+rig)
	}

	body := map[string]interface{}{
		"labels":     labels,
		"issue_type": "agent",
	}
	if !includeStopped {
		body["status"] = "open"
	}

	var issues []beadIssue
	if err := c.postBeads(ctx, "List", body, &issues); err != nil {
		return nil, 0, 0, err
	}

	var agents []Agent
	running := 0
	for _, iss := range issues {
		a := agentFromBead(iss)
		agents = append(agents, a)
		if a.State == "running" || a.State == "working" || a.State == "spawning" {
			running++
		}
	}

	return agents, len(agents), running, nil
}

// GetAgent fetches a specific agent bead by address (rig/role/name).
func (c *Client) GetAgent(ctx context.Context, agentAddr string) (*Agent, []string, error) {
	beadID := c.agentBeadID(agentAddr)

	var result struct {
		Issue beadIssue `json:"issue"`
	}
	if err := c.postBeads(ctx, "Show", map[string]interface{}{"id": beadID}, &result); err != nil {
		return nil, nil, err
	}

	agent := agentFromBead(result.Issue)
	return &agent, nil, nil
}

// SpawnPolecatRequest contains the parameters for spawning a polecat via RPC.
type SpawnPolecatRequest struct {
	Rig           string
	Name          string
	Account       string
	AgentOverride string
	HookBead      string
}

// SpawnPolecat spawns a new polecat by creating an agent bead via the beads API.
func (c *Client) SpawnPolecat(ctx context.Context, req SpawnPolecatRequest) (*Agent, string, error) {
	name := req.Name
	beadID := fmt.Sprintf("hq-%s-polecat-%s", req.Rig, name)
	if c.townName != "" {
		beadID = fmt.Sprintf("hq-%s-%s-polecat-%s", c.townName, req.Rig, name)
	}

	title := fmt.Sprintf("Polecat %s in %s", name, req.Rig)
	labels := []string{"gt:agent", "role:polecat", fmt.Sprintf("rig:%s", req.Rig), fmt.Sprintf("agent:%s", name)}
	description := fmt.Sprintf("Polecat agent %s in %s - ephemeral worker.", name, req.Rig)

	createBody := map[string]interface{}{
		"id":          beadID,
		"title":       title,
		"description": description,
		"issue_type":  "agent",
		"labels":      labels,
		"pinned":      true,
		"role_type":   "polecat",
		"rig":         req.Rig,
	}

	var created beadIssue
	if err := c.postBeads(ctx, "Create", createBody, &created); err != nil {
		return nil, "", fmt.Errorf("creating polecat bead: %w", err)
	}

	// Set status to in_progress to trigger the controller.
	updateBody := map[string]interface{}{
		"id":     beadID,
		"status": "in_progress",
	}
	if err := c.postBeads(ctx, "Update", updateBody, nil); err != nil {
		return nil, "", fmt.Errorf("setting polecat status: %w", err)
	}

	agent := &Agent{
		Address: fmt.Sprintf("%s/polecat/%s", req.Rig, name),
		Name:    name,
		Rig:     req.Rig,
		Type:    "polecat",
		State:   "running",
	}

	return agent, "", nil
}

// CreateCrewRequest contains the parameters for creating a crew workspace via RPC.
type CreateCrewRequest struct {
	Name   string
	Rig    string
	Branch bool
}

// CreateCrewResponse contains the result of creating a crew workspace.
type CreateCrewResponse struct {
	BeadID   string
	Agent    *Agent
	Reopened bool
}

// CreateCrew creates a crew workspace by writing an agent bead via the beads API.
// The K8s controller watches for the bead event and creates the crew pod.
func (c *Client) CreateCrew(ctx context.Context, req CreateCrewRequest) (*CreateCrewResponse, error) {
	beadID := crewBeadID(c.townName, req.Rig, req.Name)
	title := fmt.Sprintf("Crew worker %s in %s", req.Name, req.Rig)
	labels := []string{"gt:agent", "role:crew", fmt.Sprintf("rig:%s", req.Rig), fmt.Sprintf("agent:%s", req.Name)}
	description := fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", req.Name, req.Rig)

	createBody := map[string]interface{}{
		"id":          beadID,
		"title":       title,
		"description": description,
		"issue_type":  "agent",
		"labels":      labels,
		"pinned":      true,
		"role_type":   "crew",
		"rig":         req.Rig,
	}

	var created beadIssue
	err := c.postBeads(ctx, "Create", createBody, &created)

	reopened := false
	if err != nil {
		// If the bead already exists (UNIQUE constraint), try to reopen it.
		errStr := err.Error()
		if strings.Contains(errStr, "UNIQUE") || strings.Contains(errStr, "Duplicate") || strings.Contains(errStr, "already exists") {
			updateBody := map[string]interface{}{
				"id":     beadID,
				"status": "open",
			}
			if err2 := c.postBeads(ctx, "Update", updateBody, nil); err2 != nil {
				return nil, fmt.Errorf("reopening existing crew bead: %w", err2)
			}
			reopened = true
		} else {
			return nil, fmt.Errorf("creating crew bead: %w", err)
		}
	}

	// Set status to in_progress to trigger the controller.
	updateBody := map[string]interface{}{
		"id":     beadID,
		"status": "in_progress",
	}
	if err := c.postBeads(ctx, "Update", updateBody, nil); err != nil {
		return nil, fmt.Errorf("setting crew status to in_progress: %w", err)
	}

	agent := &Agent{
		Address: fmt.Sprintf("%s/crew/%s", req.Rig, req.Name),
		Name:    req.Name,
		Rig:     req.Rig,
		Type:    "crew",
		State:   "running",
	}

	return &CreateCrewResponse{
		BeadID:   beadID,
		Agent:    agent,
		Reopened: reopened,
	}, nil
}

// RemoveCrewRequest contains the parameters for removing a crew workspace via RPC.
type RemoveCrewRequest struct {
	Name   string
	Rig    string
	Purge  bool
	Force  bool
	Reason string
}

// RemoveCrewResponse contains the result of removing a crew workspace.
type RemoveCrewResponse struct {
	BeadID  string
	Deleted bool
}

// RemoveCrew removes a crew workspace by closing or deleting the agent bead.
// The controller reacts to the bead close/delete event to remove the pod.
func (c *Client) RemoveCrew(ctx context.Context, req RemoveCrewRequest) (*RemoveCrewResponse, error) {
	beadID := crewBeadID(c.townName, req.Rig, req.Name)

	reason := req.Reason
	if reason == "" {
		reason = "crew removed via CLI"
	}

	deleted := false
	if req.Purge {
		// Add purge label so controller knows to delete PVC too.
		_ = c.postBeads(ctx, "LabelAdd", map[string]interface{}{
			"id":    beadID,
			"label": "gt:purge",
		}, nil)

		if err := c.postBeads(ctx, "Delete", map[string]interface{}{
			"ids":   []string{beadID},
			"force": true,
		}, nil); err != nil {
			return nil, fmt.Errorf("deleting crew bead: %w", err)
		}
		deleted = true
	} else {
		// Soft close: controller removes pod but preserves PVC.
		if err := c.postBeads(ctx, "Close", map[string]interface{}{
			"id":     beadID,
			"reason": reason,
		}, nil); err != nil {
			return nil, fmt.Errorf("closing crew bead: %w", err)
		}
	}

	return &RemoveCrewResponse{
		BeadID:  beadID,
		Deleted: deleted,
	}, nil
}

// StartCrewRequest contains the parameters for starting a crew session via RPC.
type StartCrewRequest struct {
	Name          string
	Rig           string
	Account       string
	AgentOverride string
	Create        bool
}

// StartCrewResponse contains the result of starting a crew session.
type StartCrewResponse struct {
	Agent   *Agent
	Session string
	Created bool
}

// StartCrew starts a crew worker by reopening its agent bead and setting in_progress.
func (c *Client) StartCrew(ctx context.Context, req StartCrewRequest) (*StartCrewResponse, error) {
	beadID := crewBeadID(c.townName, req.Rig, req.Name)

	updateBody := map[string]interface{}{
		"id":     beadID,
		"status": "in_progress",
	}
	if err := c.postBeads(ctx, "Update", updateBody, nil); err != nil {
		return nil, fmt.Errorf("starting crew bead: %w", err)
	}

	agent := &Agent{
		Address: fmt.Sprintf("%s/crew/%s", req.Rig, req.Name),
		Name:    req.Name,
		Rig:     req.Rig,
		Type:    "crew",
		State:   "running",
	}

	return &StartCrewResponse{
		Agent: agent,
	}, nil
}

// StopAgent stops an agent by updating its agent_state to "stopping".
func (c *Client) StopAgent(ctx context.Context, agentAddr string, force bool, reason string) (*Agent, bool, error) {
	beadID := c.agentBeadID(agentAddr)

	updateBody := map[string]interface{}{
		"id":          beadID,
		"agent_state": "stopping",
	}
	if err := c.postBeads(ctx, "Update", updateBody, nil); err != nil {
		return nil, false, fmt.Errorf("stopping agent: %w", err)
	}

	parts := strings.Split(agentAddr, "/")
	name := agentAddr
	if len(parts) == 3 {
		name = parts[2]
	}

	agent := &Agent{
		Address: agentAddr,
		Name:    name,
		State:   "stopping",
	}

	return agent, false, nil
}

// NudgeAgent sends a message to an agent's terminal via RPC.
// NOTE: This still uses the gastown AgentService endpoint as nudge is
// a terminal operation that cannot be expressed as a bead mutation.
func (c *Client) NudgeAgent(ctx context.Context, agentAddr, message string, urgent bool) (bool, string, error) {
	body := map[string]interface{}{
		"agent":   agentAddr,
		"message": message,
	}
	if urgent {
		body["urgent"] = true
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return false, "", fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.AgentService/NudgeAgent",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return false, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		Delivered bool   `json:"delivered"`
		Session   string `json:"session"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", fmt.Errorf("decoding response: %w", err)
	}

	return result.Delivered, result.Session, nil
}

// PeekAgent returns recent terminal output from an agent via RPC.
func (c *Client) PeekAgent(ctx context.Context, agentAddr string, lines int, all bool) (string, []string, bool, error) {
	body := map[string]interface{}{
		"agent": agentAddr,
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
		c.baseURL+"/gastown.v1.AgentService/PeekAgent",
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

func agentTypeToProto(t string) string {
	switch t {
	case "crew":
		return "AGENT_TYPE_CREW"
	case "polecat":
		return "AGENT_TYPE_POLECAT"
	case "witness":
		return "AGENT_TYPE_WITNESS"
	case "refinery":
		return "AGENT_TYPE_REFINERY"
	case "mayor":
		return "AGENT_TYPE_MAYOR"
	case "deacon":
		return "AGENT_TYPE_DEACON"
	default:
		return "AGENT_TYPE_UNSPECIFIED"
	}
}

func agentTypeToString(t string) string {
	switch t {
	case "AGENT_TYPE_CREW":
		return "crew"
	case "AGENT_TYPE_POLECAT":
		return "polecat"
	case "AGENT_TYPE_WITNESS":
		return "witness"
	case "AGENT_TYPE_REFINERY":
		return "refinery"
	case "AGENT_TYPE_MAYOR":
		return "mayor"
	case "AGENT_TYPE_DEACON":
		return "deacon"
	default:
		if t != "" && !strings.HasPrefix(t, "AGENT_TYPE_") {
			return t
		}
		return "unknown"
	}
}

func agentStateToString(s string) string {
	switch s {
	case "AGENT_STATE_RUNNING":
		return "running"
	case "AGENT_STATE_STOPPED":
		return "stopped"
	case "AGENT_STATE_WORKING":
		return "working"
	case "AGENT_STATE_IDLE":
		return "idle"
	case "AGENT_STATE_STUCK":
		return "stuck"
	case "AGENT_STATE_DONE":
		return "done"
	default:
		if s != "" && !strings.HasPrefix(s, "AGENT_STATE_") {
			return s
		}
		return "unknown"
	}
}

// ============================================================================
// BeadsService Client Methods
// ============================================================================

// Issue represents a beads issue from the RPC API.
type Issue struct {
	ID              string
	Title           string
	Description     string
	Status          string
	Priority        int
	Type            string
	CreatedAt       string
	UpdatedAt       string
	ClosedAt        string
	Parent          string
	Assignee        string
	CreatedBy       string
	Labels          []string
	Children        []string
	DependsOn       []string
	Blocks          []string
	BlockedBy       []string
	DependencyCount int
	DependentCount  int
	BlockedByCount  int
	HookBead        string
	AgentState      string
}

// ListIssuesRequest contains the parameters for listing issues via RPC.
type ListIssuesRequest struct {
	Status     string
	Type       string
	Label      string
	Priority   int
	Parent     string
	Assignee   string
	NoAssignee bool
	Limit      int
	Offset     int
}

// ListIssues fetches issues from the RPC server.
func (c *Client) ListIssues(ctx context.Context, req ListIssuesRequest) ([]Issue, int, error) {
	body := map[string]interface{}{}
	if req.Status != "" {
		body["status"] = req.Status
	}
	if req.Type != "" {
		body["type"] = issueTypeToProto(req.Type)
	}
	if req.Label != "" {
		body["label"] = req.Label
	}
	if req.Priority >= 0 {
		body["priority"] = req.Priority
	}
	if req.Parent != "" {
		body["parent"] = req.Parent
	}
	if req.Assignee != "" {
		body["assignee"] = req.Assignee
	}
	if req.NoAssignee {
		body["noAssignee"] = true
	}
	if req.Limit > 0 {
		body["limit"] = req.Limit
	}
	if req.Offset > 0 {
		body["offset"] = req.Offset
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.BeadsService/ListIssues",
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
		Issues []issueJSON `json:"issues"`
		Total  int         `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	var issues []Issue
	for _, i := range result.Issues {
		issues = append(issues, issueFromJSON(i))
	}

	return issues, result.Total, nil
}

// GetIssue fetches a specific issue by ID via RPC.
func (c *Client) GetIssue(ctx context.Context, issueID string) (*Issue, error) {
	body := map[string]interface{}{
		"id": issueID,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.BeadsService/GetIssue",
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
		Issue issueJSON `json:"issue"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	issue := issueFromJSON(result.Issue)
	return &issue, nil
}

// CreateIssueRequest contains the parameters for creating an issue via RPC.
type CreateIssueRequest struct {
	Title       string
	Type        string
	Priority    int
	Description string
	Parent      string
	Assignee    string
	Labels      []string
	Actor       string
	ID          string
	Ephemeral   bool
}

// CreateIssue creates a new issue via RPC.
func (c *Client) CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error) {
	body := map[string]interface{}{
		"title": req.Title,
	}
	if req.Type != "" {
		body["type"] = issueTypeToProto(req.Type)
	}
	if req.Priority >= 0 {
		body["priority"] = req.Priority
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.Parent != "" {
		body["parent"] = req.Parent
	}
	if req.Assignee != "" {
		body["assignee"] = req.Assignee
	}
	if len(req.Labels) > 0 {
		body["labels"] = req.Labels
	}
	if req.Actor != "" {
		body["actor"] = req.Actor
	}
	if req.ID != "" {
		body["id"] = req.ID
	}
	if req.Ephemeral {
		body["ephemeral"] = true
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.BeadsService/CreateIssue",
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
		Issue issueJSON `json:"issue"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	issue := issueFromJSON(result.Issue)
	return &issue, nil
}

// CloseIssues closes one or more issues via RPC.
func (c *Client) CloseIssues(ctx context.Context, ids []string, reason string) (int, []string, error) {
	body := map[string]interface{}{
		"ids": ids,
	}
	if reason != "" {
		body["reason"] = reason
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.BeadsService/CloseIssues",
		strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("X-GT-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("RPC error: %s", resp.Status)
	}

	var result struct {
		ClosedCount int      `json:"closedCount"`
		FailedIds   []string `json:"failedIds"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.ClosedCount, result.FailedIds, nil
}

// SearchIssues searches issues by text query via RPC.
func (c *Client) SearchIssues(ctx context.Context, query string, status, issueType, label, assignee string, limit int) ([]Issue, int, error) {
	body := map[string]interface{}{
		"query": query,
	}
	if status != "" {
		body["status"] = status
	}
	if issueType != "" {
		body["type"] = issueTypeToProto(issueType)
	}
	if label != "" {
		body["label"] = label
	}
	if assignee != "" {
		body["assignee"] = assignee
	}
	if limit > 0 {
		body["limit"] = limit
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.BeadsService/SearchIssues",
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
		Issues []issueJSON `json:"issues"`
		Total  int         `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	var issues []Issue
	for _, i := range result.Issues {
		issues = append(issues, issueFromJSON(i))
	}

	return issues, result.Total, nil
}

// GetReadyIssues returns issues ready to work (not blocked) via RPC.
func (c *Client) GetReadyIssues(ctx context.Context, label string, limit int) ([]Issue, int, error) {
	body := map[string]interface{}{}
	if label != "" {
		body["label"] = label
	}
	if limit > 0 {
		body["limit"] = limit
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/gastown.v1.BeadsService/GetReadyIssues",
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
		Issues []issueJSON `json:"issues"`
		Total  int         `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding response: %w", err)
	}

	var issues []Issue
	for _, i := range result.Issues {
		issues = append(issues, issueFromJSON(i))
	}

	return issues, result.Total, nil
}

// Helper types and functions for JSON parsing

type issueJSON struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	Priority        int      `json:"priority"`
	Type            string   `json:"type"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
	ClosedAt        string   `json:"closedAt"`
	Parent          string   `json:"parent"`
	Assignee        string   `json:"assignee"`
	CreatedBy       string   `json:"createdBy"`
	Labels          []string `json:"labels"`
	Children        []string `json:"children"`
	DependsOn       []string `json:"dependsOn"`
	Blocks          []string `json:"blocks"`
	BlockedBy       []string `json:"blockedBy"`
	DependencyCount int      `json:"dependencyCount"`
	DependentCount  int      `json:"dependentCount"`
	BlockedByCount  int      `json:"blockedByCount"`
	HookBead        string   `json:"hookBead"`
	AgentState      string   `json:"agentState"`
}

func issueFromJSON(j issueJSON) Issue {
	return Issue{
		ID:              j.ID,
		Title:           j.Title,
		Description:     j.Description,
		Status:          issueStatusToString(j.Status),
		Priority:        j.Priority,
		Type:            issueTypeToString(j.Type),
		CreatedAt:       j.CreatedAt,
		UpdatedAt:       j.UpdatedAt,
		ClosedAt:        j.ClosedAt,
		Parent:          j.Parent,
		Assignee:        j.Assignee,
		CreatedBy:       j.CreatedBy,
		Labels:          j.Labels,
		Children:        j.Children,
		DependsOn:       j.DependsOn,
		Blocks:          j.Blocks,
		BlockedBy:       j.BlockedBy,
		DependencyCount: j.DependencyCount,
		DependentCount:  j.DependentCount,
		BlockedByCount:  j.BlockedByCount,
		HookBead:        j.HookBead,
		AgentState:      j.AgentState,
	}
}

func issueTypeToProto(t string) string {
	switch t {
	case "task":
		return "ISSUE_TYPE_TASK"
	case "bug":
		return "ISSUE_TYPE_BUG"
	case "feature":
		return "ISSUE_TYPE_FEATURE"
	case "epic":
		return "ISSUE_TYPE_EPIC"
	case "chore":
		return "ISSUE_TYPE_CHORE"
	case "merge-request":
		return "ISSUE_TYPE_MERGE_REQUEST"
	case "molecule":
		return "ISSUE_TYPE_MOLECULE"
	case "gate":
		return "ISSUE_TYPE_GATE"
	case "message":
		return "ISSUE_TYPE_MESSAGE"
	case "decision":
		return "ISSUE_TYPE_DECISION"
	case "convoy":
		return "ISSUE_TYPE_CONVOY"
	default:
		return "ISSUE_TYPE_UNSPECIFIED"
	}
}

func issueTypeToString(t string) string {
	switch t {
	case "ISSUE_TYPE_TASK":
		return "task"
	case "ISSUE_TYPE_BUG":
		return "bug"
	case "ISSUE_TYPE_FEATURE":
		return "feature"
	case "ISSUE_TYPE_EPIC":
		return "epic"
	case "ISSUE_TYPE_CHORE":
		return "chore"
	case "ISSUE_TYPE_MERGE_REQUEST":
		return "merge-request"
	case "ISSUE_TYPE_MOLECULE":
		return "molecule"
	case "ISSUE_TYPE_GATE":
		return "gate"
	case "ISSUE_TYPE_MESSAGE":
		return "message"
	case "ISSUE_TYPE_DECISION":
		return "decision"
	case "ISSUE_TYPE_CONVOY":
		return "convoy"
	default:
		if t != "" && !strings.HasPrefix(t, "ISSUE_TYPE_") {
			return t
		}
		return "unknown"
	}
}

func issueStatusToString(s string) string {
	switch s {
	case "ISSUE_STATUS_OPEN":
		return "open"
	case "ISSUE_STATUS_IN_PROGRESS":
		return "in_progress"
	case "ISSUE_STATUS_BLOCKED":
		return "blocked"
	case "ISSUE_STATUS_DEFERRED":
		return "deferred"
	case "ISSUE_STATUS_CLOSED":
		return "closed"
	default:
		if s != "" && !strings.HasPrefix(s, "ISSUE_STATUS_") {
			return s
		}
		return "unknown"
	}
}
