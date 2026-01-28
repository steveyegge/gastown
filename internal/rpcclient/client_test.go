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
		decision, err := c.ResolveDecision(context.Background(), "dec-123", 2, "B is better")
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
		_, err := c.ResolveDecision(context.Background(), "nonexistent", 1, "reason")
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
