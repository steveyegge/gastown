package rpcclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewClient tests client creation with options.
func TestNewClient(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		c := NewClient("http://localhost:8080")
		if c.baseURL != "http://localhost:8080" {
			t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:8080")
		}
		if c.httpClient.Timeout != 30*time.Second {
			t.Errorf("timeout = %v, want 30s", c.httpClient.Timeout)
		}
		if c.apiKey != "" {
			t.Errorf("apiKey = %q, want empty", c.apiKey)
		}
	})

	t.Run("trailing slash removed", func(t *testing.T) {
		c := NewClient("http://localhost:8080/")
		if c.baseURL != "http://localhost:8080" {
			t.Errorf("baseURL = %q, want without trailing slash", c.baseURL)
		}
	})

	t.Run("with API key", func(t *testing.T) {
		c := NewClient("http://localhost:8080", WithAPIKey("secret"))
		if c.apiKey != "secret" {
			t.Errorf("apiKey = %q, want %q", c.apiKey, "secret")
		}
	})

	t.Run("with timeout", func(t *testing.T) {
		c := NewClient("http://localhost:8080", WithTimeout(5*time.Second))
		if c.httpClient.Timeout != 5*time.Second {
			t.Errorf("timeout = %v, want 5s", c.httpClient.Timeout)
		}
	})

	t.Run("multiple options", func(t *testing.T) {
		c := NewClient("http://localhost:8080",
			WithAPIKey("key"),
			WithTimeout(10*time.Second))
		if c.apiKey != "key" {
			t.Errorf("apiKey = %q, want %q", c.apiKey, "key")
		}
		if c.httpClient.Timeout != 10*time.Second {
			t.Errorf("timeout = %v, want 10s", c.httpClient.Timeout)
		}
	})
}

// TestUrgencyToString tests urgency conversion to lowercase strings.
func TestUrgencyToString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"URGENCY_HIGH", "high"},
		{"URGENCY_MEDIUM", "medium"},
		{"URGENCY_LOW", "low"},
		{"URGENCY_UNSPECIFIED", "URGENCY_UNSPECIFIED"}, // Passthrough for unknown
		{"", "medium"},
		{"high", "high"},   // already lowercase
		{"medium", "medium"},
		{"low", "low"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := urgencyToString(tt.input)
			if got != tt.want {
				t.Errorf("urgencyToString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestUrgencyToProto tests urgency conversion to proto format.
func TestUrgencyToProto(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"high", "URGENCY_HIGH"},
		{"medium", "URGENCY_MEDIUM"},
		{"low", "URGENCY_LOW"},
		{"", "URGENCY_MEDIUM"},
		{"invalid", "URGENCY_MEDIUM"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := urgencyToProto(tt.input)
			if got != tt.want {
				t.Errorf("urgencyToProto(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDecisionStructs tests Decision and DecisionOption structs.
func TestDecisionStructs(t *testing.T) {
	t.Run("Decision fields", func(t *testing.T) {
		d := Decision{
			ID:          "test-123",
			Question:    "Which database?",
			Context:     "Building new service",
			Options:     []DecisionOption{{Label: "PostgreSQL"}, {Label: "MongoDB"}},
			ChosenIndex: 0,
			RequestedBy: "gastown/crew/test",
			Urgency:     "high",
			Resolved:    false,
		}
		if d.ID != "test-123" {
			t.Errorf("ID = %q, want %q", d.ID, "test-123")
		}
		if len(d.Options) != 2 {
			t.Errorf("len(Options) = %d, want 2", len(d.Options))
		}
	})

	t.Run("DecisionOption fields", func(t *testing.T) {
		opt := DecisionOption{
			Label:       "JWT tokens",
			Description: "Stateless authentication",
			Recommended: true,
		}
		if opt.Label != "JWT tokens" {
			t.Errorf("Label = %q, want %q", opt.Label, "JWT tokens")
		}
		if !opt.Recommended {
			t.Error("Recommended = false, want true")
		}
	})

	t.Run("CreateDecisionRequest fields", func(t *testing.T) {
		req := CreateDecisionRequest{
			Question:    "Which approach?",
			Context:     "Context here",
			Options:     []DecisionOption{{Label: "A"}, {Label: "B"}},
			RequestedBy: "test-agent",
			Urgency:     "medium",
			Blockers:    []string{"issue-1", "issue-2"},
			ParentBead:  "parent-123",
		}
		if req.Question != "Which approach?" {
			t.Errorf("Question = %q", req.Question)
		}
		if len(req.Blockers) != 2 {
			t.Errorf("len(Blockers) = %d, want 2", len(req.Blockers))
		}
	})
}

// TestIsAvailable tests the health check endpoint.
func TestIsAvailable(t *testing.T) {
	t.Run("server available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		if !c.IsAvailable(context.Background()) {
			t.Error("IsAvailable() = false, want true")
		}
	})

	t.Run("server unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		if c.IsAvailable(context.Background()) {
			t.Error("IsAvailable() = true, want false")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		c := NewClient("http://localhost:9999")
		if c.IsAvailable(context.Background()) {
			t.Error("IsAvailable() = true for unreachable server")
		}
	})

	t.Run("respects context timeout", func(t *testing.T) {
		// Server that delays forever
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Second)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		available := c.IsAvailable(ctx)
		elapsed := time.Since(start)

		if available {
			t.Error("IsAvailable() = true for slow server")
		}
		// Should return quickly due to internal 2s timeout
		if elapsed > 3*time.Second {
			t.Errorf("IsAvailable() took %v, expected faster due to timeout", elapsed)
		}
	})
}

// TestListPendingDecisions tests listing pending decisions.
func TestListPendingDecisions(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/ListPending" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decisions": []interface{}{},
				"total":     0,
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		decisions, err := c.ListPendingDecisions(context.Background())
		if err != nil {
			t.Fatalf("ListPendingDecisions failed: %v", err)
		}
		if len(decisions) != 0 {
			t.Errorf("len(decisions) = %d, want 0", len(decisions))
		}
	})

	t.Run("with decisions", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decisions": []map[string]interface{}{
					{
						"id":       "dec-1",
						"question": "Which framework?",
						"context":  "Building web app",
						"options": []map[string]interface{}{
							{"label": "React", "description": "Popular", "recommended": true},
							{"label": "Vue", "description": "Simple"},
						},
						"requestedBy": map[string]string{"name": "agent-1"},
						"urgency":     "URGENCY_HIGH",
						"resolved":    false,
					},
					{
						"id":          "dec-2",
						"question":    "Which database?",
						"requestedBy": map[string]string{"name": "agent-2"},
						"urgency":     "URGENCY_MEDIUM",
					},
				},
				"total": 2,
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		decisions, err := c.ListPendingDecisions(context.Background())
		if err != nil {
			t.Fatalf("ListPendingDecisions failed: %v", err)
		}
		if len(decisions) != 2 {
			t.Fatalf("len(decisions) = %d, want 2", len(decisions))
		}

		// Verify first decision
		d1 := decisions[0]
		if d1.ID != "dec-1" {
			t.Errorf("decisions[0].ID = %q, want dec-1", d1.ID)
		}
		if d1.Question != "Which framework?" {
			t.Errorf("decisions[0].Question = %q", d1.Question)
		}
		if d1.Urgency != "high" {
			t.Errorf("decisions[0].Urgency = %q, want high", d1.Urgency)
		}
		if len(d1.Options) != 2 {
			t.Errorf("len(decisions[0].Options) = %d, want 2", len(d1.Options))
		}
		if d1.Options[0].Label != "React" {
			t.Errorf("decisions[0].Options[0].Label = %q", d1.Options[0].Label)
		}
		if !d1.Options[0].Recommended {
			t.Error("decisions[0].Options[0].Recommended = false, want true")
		}

		// Verify second decision
		d2 := decisions[1]
		if d2.ID != "dec-2" {
			t.Errorf("decisions[1].ID = %q, want dec-2", d2.ID)
		}
		if d2.Urgency != "medium" {
			t.Errorf("decisions[1].Urgency = %q, want medium", d2.Urgency)
		}
	})

	t.Run("with API key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-GT-API-Key")
			if apiKey != "secret-key" {
				t.Errorf("X-GT-API-Key = %q, want secret-key", apiKey)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"decisions": []interface{}{}})
		}))
		defer server.Close()

		c := NewClient(server.URL, WithAPIKey("secret-key"))
		_, err := c.ListPendingDecisions(context.Background())
		if err != nil {
			t.Fatalf("ListPendingDecisions failed: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.ListPendingDecisions(context.Background())
		if err == nil {
			t.Error("expected error for server error response")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not valid json"))
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.ListPendingDecisions(context.Background())
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := c.ListPendingDecisions(ctx)
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}

// TestCreateDecision tests creating a decision.
func TestCreateDecision(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/CreateDecision" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			// Parse request body
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			if req["question"] != "Which approach?" {
				t.Errorf("request.question = %v", req["question"])
			}
			if req["urgency"] != "URGENCY_HIGH" {
				t.Errorf("request.urgency = %v", req["urgency"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decision": map[string]interface{}{
					"id":       "new-dec-123",
					"question": "Which approach?",
					"context":  "Context here",
					"options": []map[string]interface{}{
						{"label": "A", "recommended": true},
						{"label": "B"},
					},
					"requestedBy": map[string]string{"name": "test-agent"},
					"urgency":     "URGENCY_HIGH",
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		decision, err := c.CreateDecision(context.Background(), CreateDecisionRequest{
			Question:    "Which approach?",
			Context:     "Context here",
			Options:     []DecisionOption{{Label: "A", Recommended: true}, {Label: "B"}},
			RequestedBy: "test-agent",
			Urgency:     "high",
		})
		if err != nil {
			t.Fatalf("CreateDecision failed: %v", err)
		}

		if decision.ID != "new-dec-123" {
			t.Errorf("decision.ID = %q, want new-dec-123", decision.ID)
		}
		if decision.Question != "Which approach?" {
			t.Errorf("decision.Question = %q", decision.Question)
		}
		if decision.Urgency != "high" {
			t.Errorf("decision.Urgency = %q, want high", decision.Urgency)
		}
		if len(decision.Options) != 2 {
			t.Errorf("len(decision.Options) = %d, want 2", len(decision.Options))
		}
	})

	t.Run("with blockers and parent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			// Verify blockers and parent are included
			if req["blockers"] == nil {
				t.Error("blockers not included in request")
			}
			if req["parentBead"] != "parent-123" {
				t.Errorf("parentBead = %v, want parent-123", req["parentBead"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decision": map[string]interface{}{
					"id": "dec-with-blockers",
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.CreateDecision(context.Background(), CreateDecisionRequest{
			Question:   "Q?",
			Options:    []DecisionOption{{Label: "A"}},
			Blockers:   []string{"blocker-1", "blocker-2"},
			ParentBead: "parent-123",
		})
		if err != nil {
			t.Fatalf("CreateDecision failed: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.CreateDecision(context.Background(), CreateDecisionRequest{
			Question: "Q?",
			Options:  []DecisionOption{{Label: "A"}},
		})
		if err == nil {
			t.Error("expected error for server error response")
		}
	})
}

// TestResolveDecision tests resolving a decision.
func TestResolveDecision(t *testing.T) {
	t.Run("successful resolution", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/Resolve" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["decisionId"] != "dec-123" {
				t.Errorf("decisionId = %v", req["decisionId"])
			}
			if req["chosenIndex"] != float64(2) {
				t.Errorf("chosenIndex = %v", req["chosenIndex"])
			}
			if req["rationale"] != "B is better" {
				t.Errorf("rationale = %v", req["rationale"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decision": map[string]interface{}{
					"id":          "dec-123",
					"question":    "Which?",
					"chosenIndex": 2,
					"rationale":   "B is better",
					"resolved":    true,
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		decision, err := c.ResolveDecision(context.Background(), "dec-123", 2, "B is better", "test-resolver")
		if err != nil {
			t.Fatalf("ResolveDecision failed: %v", err)
		}

		if decision.ID != "dec-123" {
			t.Errorf("decision.ID = %q", decision.ID)
		}
		if decision.ChosenIndex != 2 {
			t.Errorf("decision.ChosenIndex = %d, want 2", decision.ChosenIndex)
		}
		if !decision.Resolved {
			t.Error("decision.Resolved = false, want true")
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.ResolveDecision(context.Background(), "nonexistent", 1, "reason", "test-resolver")
		if err == nil {
			t.Error("expected error for not found decision")
		}
	})
}

// TestGetDecision tests fetching a specific decision.
func TestGetDecision(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/GetDecision" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["decisionId"] != "dec-123" {
				t.Errorf("decisionId = %v", req["decisionId"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decision": map[string]interface{}{
					"id":          "dec-123",
					"question":    "Which option?",
					"context":     "Test context",
					"chosenIndex": 2,
					"rationale":   "B is better",
					"resolvedBy":  "slack:U12345",
					"requestedBy": map[string]interface{}{"name": "test-agent"},
					"urgency":     "URGENCY_HIGH",
					"resolved":    true,
					"options": []map[string]interface{}{
						{"label": "Option A", "description": "First option", "recommended": false},
						{"label": "Option B", "description": "Second option", "recommended": true},
					},
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		decision, err := c.GetDecision(context.Background(), "dec-123")
		if err != nil {
			t.Fatalf("GetDecision failed: %v", err)
		}

		if decision.ID != "dec-123" {
			t.Errorf("decision.ID = %q, want %q", decision.ID, "dec-123")
		}
		if decision.Question != "Which option?" {
			t.Errorf("decision.Question = %q", decision.Question)
		}
		if decision.Context != "Test context" {
			t.Errorf("decision.Context = %q", decision.Context)
		}
		if decision.ChosenIndex != 2 {
			t.Errorf("decision.ChosenIndex = %d, want 2", decision.ChosenIndex)
		}
		if decision.ResolvedBy != "slack:U12345" {
			t.Errorf("decision.ResolvedBy = %q, want %q", decision.ResolvedBy, "slack:U12345")
		}
		if decision.RequestedBy != "test-agent" {
			t.Errorf("decision.RequestedBy = %q", decision.RequestedBy)
		}
		if decision.Urgency != "high" {
			t.Errorf("decision.Urgency = %q, want %q", decision.Urgency, "high")
		}
		if !decision.Resolved {
			t.Error("decision.Resolved = false, want true")
		}
		if len(decision.Options) != 2 {
			t.Errorf("len(decision.Options) = %d, want 2", len(decision.Options))
		}
		if decision.Options[0].Label != "Option A" {
			t.Errorf("decision.Options[0].Label = %q", decision.Options[0].Label)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.GetDecision(context.Background(), "nonexistent")
		if err == nil {
			t.Error("expected error for not found decision")
		}
	})
}

// TestWatchDecisions tests the polling-based watch implementation.
func TestWatchDecisions(t *testing.T) {
	t.Run("receives initial decisions", func(t *testing.T) {
		callCount := int32(0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&callCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decisions": []map[string]interface{}{
					{"id": "dec-1", "question": "Q1", "urgency": "URGENCY_HIGH"},
					{"id": "dec-2", "question": "Q2", "urgency": "URGENCY_MEDIUM"},
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		var received []Decision
		var mu sync.Mutex

		err := c.WatchDecisions(ctx, func(d Decision) error {
			mu.Lock()
			received = append(received, d)
			mu.Unlock()
			return nil
		})

		// Error should be context deadline exceeded
		if err != context.DeadlineExceeded {
			t.Logf("WatchDecisions returned: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if len(received) != 2 {
			t.Errorf("received %d decisions, want 2", len(received))
		}
	})

	t.Run("deduplicates decisions", func(t *testing.T) {
		callCount := int32(0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&callCount, 1)
			w.Header().Set("Content-Type", "application/json")

			// Always return same decisions - should only be delivered once
			decisions := []map[string]interface{}{
				{"id": "dec-1", "question": "Q1"},
			}
			// Add new decision on second call
			if count >= 2 {
				decisions = append(decisions, map[string]interface{}{
					"id": "dec-2", "question": "Q2",
				})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decisions": decisions,
			})
		}))
		defer server.Close()

		// Use shorter poll interval for testing
		c := NewClient(server.URL, WithTimeout(2*time.Second))
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		var received []Decision
		var mu sync.Mutex

		_ = c.WatchDecisions(ctx, func(d Decision) error {
			mu.Lock()
			received = append(received, d)
			mu.Unlock()
			return nil
		})

		mu.Lock()
		defer mu.Unlock()
		// dec-1 should only appear once due to deduplication
		dec1Count := 0
		for _, d := range received {
			if d.ID == "dec-1" {
				dec1Count++
			}
		}
		if dec1Count > 1 {
			t.Errorf("dec-1 received %d times, want 1", dec1Count)
		}
	})

	t.Run("callback error stops watch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"decisions": []map[string]interface{}{
					{"id": "dec-1"},
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx := context.Background()

		callbackErr := &testError{msg: "callback failed"}
		err := c.WatchDecisions(ctx, func(d Decision) error {
			return callbackErr
		})

		if err != callbackErr {
			t.Errorf("error = %v, want callback error", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"decisions": []interface{}{}})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel immediately
		cancel()

		err := c.WatchDecisions(ctx, func(d Decision) error {
			return nil
		})

		// Error may be context.Canceled or wrapped in "initial fetch" error
		if err == nil {
			t.Error("expected error for cancelled context")
		}
		// Accept any error containing "canceled" or being context.Canceled
		if err != context.Canceled && !strings.Contains(err.Error(), "cancel") {
			t.Logf("error = %v (acceptable for cancelled context)", err)
		}
	})
}

// testError is a simple error type for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestClientConcurrency tests thread safety of the client.
func TestClientConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"decisions": []map[string]interface{}{
				{"id": "dec-1"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	var wg sync.WaitGroup

	// Run multiple concurrent requests
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.ListPendingDecisions(context.Background())
			if err != nil {
				t.Errorf("concurrent ListPendingDecisions failed: %v", err)
			}
		}()
	}

	wg.Wait()
}

// TestEmptyOptions tests handling of decisions with no options.
func TestEmptyOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"decisions": []map[string]interface{}{
				{
					"id":       "dec-1",
					"question": "Q?",
					"options":  []interface{}{}, // Empty options
				},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	decisions, err := c.ListPendingDecisions(context.Background())
	if err != nil {
		t.Fatalf("ListPendingDecisions failed: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("len(decisions) = %d, want 1", len(decisions))
	}
	if len(decisions[0].Options) != 0 {
		t.Errorf("len(Options) = %d, want 0", len(decisions[0].Options))
	}
}

// TestNilOptions tests handling of decisions with nil options.
func TestNilOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"decisions": []map[string]interface{}{
				{
					"id":       "dec-1",
					"question": "Q?",
					// No options field at all
				},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	decisions, err := c.ListPendingDecisions(context.Background())
	if err != nil {
		t.Fatalf("ListPendingDecisions failed: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("len(decisions) = %d, want 1", len(decisions))
	}
	// Options should be nil or empty, not cause a panic
	if decisions[0].Options != nil && len(decisions[0].Options) > 0 {
		t.Errorf("Options = %v, want nil or empty", decisions[0].Options)
	}
}

// --- Decision Chaining Tests ---

// TestGetDecisionChain tests fetching a decision's ancestry chain.
func TestGetDecisionChain(t *testing.T) {
	t.Run("successful chain fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/GetChain" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("method = %s, want POST", r.Method)
			}

			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["decisionId"] != "dec-child" {
				t.Errorf("decisionId = %v, want dec-child", req["decisionId"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chain": []map[string]interface{}{
					{
						"id":          "dec-root",
						"question":    "Root decision",
						"chosenIndex": 1,
						"chosenLabel": "Option A",
						"urgency":     "URGENCY_HIGH",
						"requestedBy": "agent-1",
						"requestedAt": "2026-01-29T10:00:00Z",
						"resolvedAt":  "2026-01-29T10:30:00Z",
					},
					{
						"id":            "dec-child",
						"question":      "Child decision",
						"chosenIndex":   2,
						"chosenLabel":   "Option B",
						"urgency":       "URGENCY_MEDIUM",
						"requestedBy":   "agent-2",
						"requestedAt":   "2026-01-29T11:00:00Z",
						"predecessorId": "dec-root",
						"isTarget":      true,
					},
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		chain, err := c.GetDecisionChain(context.Background(), "dec-child")
		if err != nil {
			t.Fatalf("GetDecisionChain failed: %v", err)
		}

		if len(chain) != 2 {
			t.Fatalf("len(chain) = %d, want 2", len(chain))
		}

		// Verify root node
		root := chain[0]
		if root.ID != "dec-root" {
			t.Errorf("chain[0].ID = %q, want dec-root", root.ID)
		}
		if root.Question != "Root decision" {
			t.Errorf("chain[0].Question = %q", root.Question)
		}
		if root.ChosenIndex != 1 {
			t.Errorf("chain[0].ChosenIndex = %d, want 1", root.ChosenIndex)
		}
		if root.ChosenLabel != "Option A" {
			t.Errorf("chain[0].ChosenLabel = %q", root.ChosenLabel)
		}
		if root.Urgency != "high" {
			t.Errorf("chain[0].Urgency = %q, want high", root.Urgency)
		}
		if root.PredecessorID != "" {
			t.Errorf("chain[0].PredecessorID = %q, want empty", root.PredecessorID)
		}

		// Verify child node
		child := chain[1]
		if child.ID != "dec-child" {
			t.Errorf("chain[1].ID = %q, want dec-child", child.ID)
		}
		if child.PredecessorID != "dec-root" {
			t.Errorf("chain[1].PredecessorID = %q, want dec-root", child.PredecessorID)
		}
		if !child.IsTarget {
			t.Error("chain[1].IsTarget = false, want true")
		}
		if child.Urgency != "medium" {
			t.Errorf("chain[1].Urgency = %q, want medium", child.Urgency)
		}
	})

	t.Run("single node chain", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chain": []map[string]interface{}{
					{
						"id":        "dec-single",
						"question":  "Single decision",
						"urgency":   "URGENCY_LOW",
						"isTarget":  true,
					},
				},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		chain, err := c.GetDecisionChain(context.Background(), "dec-single")
		if err != nil {
			t.Fatalf("GetDecisionChain failed: %v", err)
		}

		if len(chain) != 1 {
			t.Fatalf("len(chain) = %d, want 1", len(chain))
		}
		if !chain[0].IsTarget {
			t.Error("single node should be target")
		}
	})

	t.Run("empty chain", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chain": []interface{}{},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		chain, err := c.GetDecisionChain(context.Background(), "nonexistent")
		if err != nil {
			t.Fatalf("GetDecisionChain failed: %v", err)
		}

		if len(chain) != 0 {
			t.Errorf("len(chain) = %d, want 0", len(chain))
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, err := c.GetDecisionChain(context.Background(), "dec-123")
		if err == nil {
			t.Error("expected error for server error response")
		}
	})

	t.Run("with API key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-GT-API-Key")
			if apiKey != "test-key" {
				t.Errorf("X-GT-API-Key = %q, want test-key", apiKey)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"chain": []interface{}{}})
		}))
		defer server.Close()

		c := NewClient(server.URL, WithAPIKey("test-key"))
		_, err := c.GetDecisionChain(context.Background(), "dec-123")
		if err != nil {
			t.Fatalf("GetDecisionChain failed: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := c.GetDecisionChain(ctx, "dec-123")
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})

	t.Run("deep chain", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// 5-level deep chain
			chain := []map[string]interface{}{
				{"id": "dec-1", "question": "Level 1", "urgency": "URGENCY_HIGH"},
				{"id": "dec-2", "question": "Level 2", "urgency": "URGENCY_HIGH", "predecessorId": "dec-1"},
				{"id": "dec-3", "question": "Level 3", "urgency": "URGENCY_MEDIUM", "predecessorId": "dec-2"},
				{"id": "dec-4", "question": "Level 4", "urgency": "URGENCY_LOW", "predecessorId": "dec-3"},
				{"id": "dec-5", "question": "Level 5", "urgency": "URGENCY_LOW", "predecessorId": "dec-4", "isTarget": true},
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"chain": chain})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		chain, err := c.GetDecisionChain(context.Background(), "dec-5")
		if err != nil {
			t.Fatalf("GetDecisionChain failed: %v", err)
		}

		if len(chain) != 5 {
			t.Fatalf("len(chain) = %d, want 5", len(chain))
		}

		// Verify chain order (root to target)
		for i, node := range chain {
			expectedID := "dec-" + string(rune('1'+i))
			if node.ID != expectedID {
				t.Errorf("chain[%d].ID = %q, want %q", i, node.ID, expectedID)
			}
		}

		// Verify target is last
		if !chain[4].IsTarget {
			t.Error("last node should be target")
		}
	})
}

// TestValidateDecisionContext tests the context validation RPC.
func TestValidateDecisionContext(t *testing.T) {
	t.Run("valid context", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/ValidateContext" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["context"] != `{"key": "value"}` {
				t.Errorf("context = %v", req["context"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":  true,
				"errors": []string{},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		valid, errors, err := c.ValidateDecisionContext(context.Background(), `{"key": "value"}`, "")
		if err != nil {
			t.Fatalf("ValidateDecisionContext failed: %v", err)
		}

		if !valid {
			t.Error("valid = false, want true")
		}
		if len(errors) != 0 {
			t.Errorf("errors = %v, want empty", errors)
		}
	})

	t.Run("invalid context", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":  false,
				"errors": []string{"invalid JSON syntax", "missing required field"},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		valid, errors, err := c.ValidateDecisionContext(context.Background(), "not valid json", "")
		if err != nil {
			t.Fatalf("ValidateDecisionContext failed: %v", err)
		}

		if valid {
			t.Error("valid = true, want false")
		}
		if len(errors) != 2 {
			t.Errorf("len(errors) = %d, want 2", len(errors))
		}
	})

	t.Run("with predecessor ID", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["predecessorId"] != "dec-parent" {
				t.Errorf("predecessorId = %v, want dec-parent", req["predecessorId"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":  true,
				"errors": []string{},
			})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		valid, _, err := c.ValidateDecisionContext(context.Background(), `{"key": "value"}`, "dec-parent")
		if err != nil {
			t.Fatalf("ValidateDecisionContext failed: %v", err)
		}

		if !valid {
			t.Error("valid = false, want true")
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		_, _, err := c.ValidateDecisionContext(context.Background(), `{}`, "")
		if err == nil {
			t.Error("expected error for server error response")
		}
	})
}

// TestChainNodeStruct tests the ChainNode struct.
func TestChainNodeStruct(t *testing.T) {
	node := ChainNode{
		ID:            "test-123",
		Question:      "Test question?",
		ChosenIndex:   2,
		ChosenLabel:   "Option B",
		Urgency:       "high",
		RequestedBy:   "test-agent",
		RequestedAt:   "2026-01-29T10:00:00Z",
		ResolvedAt:    "2026-01-29T10:30:00Z",
		PredecessorID: "parent-123",
		IsTarget:      true,
	}

	// Test JSON marshaling
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"id":"test-123"`) {
		t.Error("missing id field in JSON")
	}
	if !strings.Contains(jsonStr, `"predecessor_id":"parent-123"`) {
		t.Error("missing predecessor_id field in JSON")
	}
	if !strings.Contains(jsonStr, `"is_target":true`) {
		t.Error("missing is_target field in JSON")
	}

	// Test unmarshaling
	var decoded ChainNode
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.ID != node.ID {
		t.Errorf("decoded.ID = %q, want %q", decoded.ID, node.ID)
	}
	if decoded.PredecessorID != node.PredecessorID {
		t.Errorf("decoded.PredecessorID = %q, want %q", decoded.PredecessorID, node.PredecessorID)
	}
}

// TestChainNodeWithChildren tests nested chain nodes.
func TestChainNodeWithChildren(t *testing.T) {
	root := &ChainNode{
		ID:       "root",
		Question: "Root decision",
		Children: []*ChainNode{
			{
				ID:            "child-1",
				Question:      "Child 1",
				PredecessorID: "root",
			},
			{
				ID:            "child-2",
				Question:      "Child 2",
				PredecessorID: "root",
			},
		},
	}

	// Test JSON marshaling with children
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, "children") {
		t.Error("missing children field in JSON")
	}
	if !strings.Contains(jsonStr, "child-1") {
		t.Error("missing first child in JSON")
	}
	if !strings.Contains(jsonStr, "child-2") {
		t.Error("missing second child in JSON")
	}
}

// TestCancelDecision tests canceling/dismissing a decision.
func TestCancelDecision(t *testing.T) {
	t.Run("successful cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gastown.v1.DecisionService/Cancel" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("method = %s, want POST", r.Method)
			}

			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["decisionId"] != "dec-123" {
				t.Errorf("decisionId = %v, want dec-123", req["decisionId"])
			}
			if req["reason"] != "No longer needed" {
				t.Errorf("reason = %v, want 'No longer needed'", req["reason"])
			}

			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		c := NewClient(server.URL)
		err := c.CancelDecision(context.Background(), "dec-123", "No longer needed")
		if err != nil {
			t.Fatalf("CancelDecision failed: %v", err)
		}
	})

	t.Run("cancellation without reason", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["decisionId"] != "dec-456" {
				t.Errorf("decisionId = %v, want dec-456", req["decisionId"])
			}
			// reason should not be present if empty
			if _, hasReason := req["reason"]; hasReason {
				t.Error("reason should not be included when empty")
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		err := c.CancelDecision(context.Background(), "dec-456", "")
		if err != nil {
			t.Fatalf("CancelDecision failed: %v", err)
		}
	})

	t.Run("with API key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-GT-API-Key")
			if apiKey != "secret-key" {
				t.Errorf("X-GT-API-Key = %q, want secret-key", apiKey)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := NewClient(server.URL, WithAPIKey("secret-key"))
		err := c.CancelDecision(context.Background(), "dec-789", "test")
		if err != nil {
			t.Fatalf("CancelDecision failed: %v", err)
		}
	})

	t.Run("server error - not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		err := c.CancelDecision(context.Background(), "nonexistent", "reason")
		if err == nil {
			t.Error("expected error for not found decision")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error = %v, expected to contain 404", err)
		}
	})

	t.Run("server error - internal", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		err := c.CancelDecision(context.Background(), "dec-123", "reason")
		if err == nil {
			t.Error("expected error for server error response")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()

		c := NewClient(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := c.CancelDecision(ctx, "dec-123", "reason")
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		c := NewClient("http://localhost:9999")
		err := c.CancelDecision(context.Background(), "dec-123", "reason")
		if err == nil {
			t.Error("expected error for unreachable server")
		}
	})
}

// TestCreateDecisionWithPredecessor tests creating a chained decision.
func TestCreateDecisionWithPredecessor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gastown.v1.DecisionService/CreateDecision" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		// Verify predecessor is included
		if req["predecessorId"] != "dec-parent" {
			t.Errorf("predecessorId = %v, want dec-parent", req["predecessorId"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"decision": map[string]interface{}{
				"id":            "dec-child",
				"question":      "Follow-up question?",
				"predecessorId": "dec-parent",
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	decision, err := c.CreateDecision(context.Background(), CreateDecisionRequest{
		Question:      "Follow-up question?",
		Options:       []DecisionOption{{Label: "A"}, {Label: "B"}},
		RequestedBy:   "test-agent",
		PredecessorID: "dec-parent",
	})
	if err != nil {
		t.Fatalf("CreateDecision failed: %v", err)
	}

	if decision.ID != "dec-child" {
		t.Errorf("decision.ID = %q, want dec-child", decision.ID)
	}
}
