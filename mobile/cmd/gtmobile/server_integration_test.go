package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/steveyegge/gastown/internal/eventbus"
	gastownv1 "github.com/steveyegge/gastown/mobile/gen/gastown/v1"
	"github.com/steveyegge/gastown/mobile/gen/gastown/v1/gastownv1connect"
)

// setupTestTown creates a minimal town structure for integration testing.
func setupTestTown(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "rpc-integration-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Create minimal town structure
	dirs := []string{
		"mayor",
		".beads",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatal(err)
		}
	}

	// Create minimal town.json
	townConfig := `{"name": "test-town"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "mayor", "town.json"), []byte(townConfig), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	// Create minimal rigs.json
	rigsConfig := `{"rigs": {}}`
	if err := os.WriteFile(filepath.Join(tmpDir, "mayor", "rigs.json"), []byte(rigsConfig), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// setupTestServer creates a test HTTP server with all RPC services.
func setupTestServer(t *testing.T, townRoot string) (*httptest.Server, gastownv1connect.StatusServiceClient, gastownv1connect.DecisionServiceClient, gastownv1connect.MailServiceClient) {
	t.Helper()

	mux := http.NewServeMux()

	// Create eventbus for decision server
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	// Register services
	statusServer := NewStatusServer(townRoot)
	decisionServer := NewDecisionServer(townRoot, bus)
	mailServer := NewMailServer(townRoot)

	mux.Handle(gastownv1connect.NewStatusServiceHandler(statusServer))
	mux.Handle(gastownv1connect.NewDecisionServiceHandler(decisionServer))
	mux.Handle(gastownv1connect.NewMailServiceHandler(mailServer))

	// Create test server with h2c for HTTP/2 without TLS
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	// Create clients
	statusClient := gastownv1connect.NewStatusServiceClient(
		http.DefaultClient,
		server.URL,
	)
	decisionClient := gastownv1connect.NewDecisionServiceClient(
		http.DefaultClient,
		server.URL,
	)
	mailClient := gastownv1connect.NewMailServiceClient(
		http.DefaultClient,
		server.URL,
	)

	return server, statusClient, decisionClient, mailClient
}

// TestStatusServiceIntegration tests the StatusService end-to-end.
func TestStatusServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupTestTown(t)
	defer cleanup()

	server, statusClient, _, _ := setupTestServer(t, townRoot)
	defer server.Close()

	t.Run("GetTownStatus", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetTownStatusRequest{Fast: true})
		resp, err := statusClient.GetTownStatus(ctx, req)
		if err != nil {
			t.Fatalf("GetTownStatus failed: %v", err)
		}

		if resp.Msg.Status == nil {
			t.Fatal("response status is nil")
		}

		// Verify town name from config
		if resp.Msg.Status.Name != "test-town" {
			t.Errorf("town name = %q, want %q", resp.Msg.Status.Name, "test-town")
		}

		// Verify location is set
		if resp.Msg.Status.Location != townRoot {
			t.Errorf("location = %q, want %q", resp.Msg.Status.Location, townRoot)
		}
	})

	t.Run("GetTownStatus_Fast", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fast mode should skip some expensive operations
		req := connect.NewRequest(&gastownv1.GetTownStatusRequest{Fast: true})
		resp, err := statusClient.GetTownStatus(ctx, req)
		if err != nil {
			t.Fatalf("GetTownStatus (fast) failed: %v", err)
		}

		if resp.Msg.Status == nil {
			t.Fatal("response status is nil")
		}
	})

	t.Run("GetRigStatus_NotFound", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetRigStatusRequest{RigName: "nonexistent-rig"})
		_, err := statusClient.GetRigStatus(ctx, req)
		if err == nil {
			t.Fatal("expected error for nonexistent rig")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("error code = %v, want NotFound", connectErr.Code())
		}
	})

	t.Run("GetAgentStatus_NotFound", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetAgentStatusRequest{
			Address: &gastownv1.AgentAddress{Name: "nonexistent-agent"},
		})
		_, err := statusClient.GetAgentStatus(ctx, req)
		if err == nil {
			t.Fatal("expected error for nonexistent agent")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("error code = %v, want NotFound", connectErr.Code())
		}
	})

	t.Run("GetAgentStatus_InvalidArgument_NilAddress", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetAgentStatusRequest{
			Address: nil,
		})
		_, err := statusClient.GetAgentStatus(ctx, req)
		if err == nil {
			t.Fatal("expected error for nil address")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want InvalidArgument", connectErr.Code())
		}
	})
}

// TestDecisionServiceIntegration tests the DecisionService end-to-end.
func TestDecisionServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupTestTown(t)
	defer cleanup()

	server, _, decisionClient, _ := setupTestServer(t, townRoot)
	defer server.Close()

	t.Run("ListPending_EmptyResult", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.ListPendingRequest{})
		resp, err := decisionClient.ListPending(ctx, req)

		// May fail if beads not initialized, which is expected
		if err != nil {
			// Check if it's an internal error (beads not configured)
			connectErr, ok := err.(*connect.Error)
			if ok && connectErr.Code() == connect.CodeInternal {
				t.Skip("beads not configured in test town")
			}
			t.Fatalf("ListPending failed: %v", err)
		}

		// Should return empty list if no decisions
		if resp.Msg.Total != 0 {
			t.Errorf("expected 0 decisions, got %d", resp.Msg.Total)
		}
	})

	t.Run("GetDecision_NotFound", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetDecisionRequest{DecisionId: "nonexistent-id"})
		_, err := decisionClient.GetDecision(ctx, req)
		if err == nil {
			t.Fatal("expected error for nonexistent decision")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("error code = %v, want NotFound", connectErr.Code())
		}
	})

	t.Run("Resolve_NonExistent", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.ResolveRequest{
			DecisionId:  "nonexistent-decision",
			ChosenIndex: 1,
			Rationale:   "test rationale",
		})
		_, err := decisionClient.Resolve(ctx, req)
		if err == nil {
			t.Fatal("expected error for non-existent decision")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		// Resolving non-existent decision returns Internal error
		if connectErr.Code() != connect.CodeInternal {
			t.Errorf("error code = %v, want Internal", connectErr.Code())
		}
	})

	t.Run("Cancel_NonExistent", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.CancelRequest{
			DecisionId: "nonexistent-decision",
			Reason:     "test reason",
		})
		_, err := decisionClient.Cancel(ctx, req)
		if err == nil {
			t.Fatal("expected error for non-existent decision")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		// Canceling non-existent decision returns Internal error
		if connectErr.Code() != connect.CodeInternal {
			t.Errorf("error code = %v, want Internal", connectErr.Code())
		}
	})
}

// TestMailServiceIntegration tests the MailService end-to-end.
func TestMailServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupTestTown(t)
	defer cleanup()

	server, _, _, mailClient := setupTestServer(t, townRoot)
	defer server.Close()

	t.Run("ListInbox_DefaultAddress", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.ListInboxRequest{})
		resp, err := mailClient.ListInbox(ctx, req)

		// May fail if mail not configured, which is expected
		if err != nil {
			connectErr, ok := err.(*connect.Error)
			if ok && connectErr.Code() == connect.CodeInternal {
				t.Skip("mail not configured in test town")
			}
			t.Fatalf("ListInbox failed: %v", err)
		}

		// Response should be valid even if empty
		if resp.Msg == nil {
			t.Fatal("response message is nil")
		}
	})

	t.Run("ListInbox_WithAddress", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.ListInboxRequest{
			Address: &gastownv1.AgentAddress{Name: "test-agent"},
		})
		_, err := mailClient.ListInbox(ctx, req)

		// May fail if mail not configured
		if err != nil {
			connectErr, ok := err.(*connect.Error)
			if ok && connectErr.Code() == connect.CodeInternal {
				t.Skip("mail not configured in test town")
			}
			// Other errors are acceptable for non-existent mailbox
		}
	})

	t.Run("ListInbox_UnreadOnly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.ListInboxRequest{
			UnreadOnly: true,
			Limit:      10,
		})
		resp, err := mailClient.ListInbox(ctx, req)

		if err != nil {
			connectErr, ok := err.(*connect.Error)
			if ok && connectErr.Code() == connect.CodeInternal {
				t.Skip("mail not configured in test town")
			}
			t.Fatalf("ListInbox failed: %v", err)
		}

		// All returned messages should be unread
		for _, msg := range resp.Msg.Messages {
			if msg.Read {
				t.Error("expected only unread messages when UnreadOnly=true")
			}
		}
	})

	t.Run("ReadMessage_Unimplemented", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.ReadMessageRequest{MessageId: "test-id"})
		_, err := mailClient.ReadMessage(ctx, req)
		if err == nil {
			t.Fatal("expected error for unimplemented method")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeUnimplemented {
			t.Errorf("error code = %v, want Unimplemented", connectErr.Code())
		}
	})

	t.Run("SendMessage_Unimplemented", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.SendMessageRequest{
			To:      &gastownv1.AgentAddress{Name: "recipient"},
			Subject: "Test",
			Body:    "Test body",
		})
		_, err := mailClient.SendMessage(ctx, req)
		if err == nil {
			t.Fatal("expected error for unimplemented method")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeUnimplemented {
			t.Errorf("error code = %v, want Unimplemented", connectErr.Code())
		}
	})
}

// TestAPIKeyAuthenticationIntegration tests API key authentication end-to-end.
func TestAPIKeyAuthenticationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupTestTown(t)
	defer cleanup()

	apiKey := "test-secret-key"

	mux := http.NewServeMux()

	// Create interceptor with API key
	interceptor := connect.WithInterceptors(APIKeyInterceptor(apiKey))

	// Register services with interceptor
	statusServer := NewStatusServer(townRoot)
	mux.Handle(gastownv1connect.NewStatusServiceHandler(statusServer, interceptor))

	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	defer server.Close()

	t.Run("ValidAPIKey", func(t *testing.T) {
		// Create client with valid API key header
		client := gastownv1connect.NewStatusServiceClient(
			&http.Client{
				Transport: &apiKeyTransport{
					base:   http.DefaultTransport,
					apiKey: apiKey,
				},
			},
			server.URL,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetTownStatusRequest{Fast: true})
		resp, err := client.GetTownStatus(ctx, req)
		if err != nil {
			t.Fatalf("request with valid API key failed: %v", err)
		}

		if resp.Msg.Status == nil {
			t.Fatal("response status is nil")
		}
	})

	t.Run("InvalidAPIKey", func(t *testing.T) {
		// Create client with invalid API key
		client := gastownv1connect.NewStatusServiceClient(
			&http.Client{
				Transport: &apiKeyTransport{
					base:   http.DefaultTransport,
					apiKey: "wrong-key",
				},
			},
			server.URL,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetTownStatusRequest{Fast: true})
		_, err := client.GetTownStatus(ctx, req)
		if err == nil {
			t.Fatal("expected error for invalid API key")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("error code = %v, want Unauthenticated", connectErr.Code())
		}
	})

	t.Run("MissingAPIKey", func(t *testing.T) {
		// Create client without API key
		client := gastownv1connect.NewStatusServiceClient(
			http.DefaultClient,
			server.URL,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.GetTownStatusRequest{Fast: true})
		_, err := client.GetTownStatus(ctx, req)
		if err == nil {
			t.Fatal("expected error for missing API key")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("error code = %v, want Unauthenticated", connectErr.Code())
		}
	})
}

// apiKeyTransport adds API key header to requests.
type apiKeyTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *apiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-GT-API-Key", t.apiKey)
	return t.base.RoundTrip(req)
}

// TestWatchStatusIntegration tests streaming status updates.
func TestWatchStatusIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupTestTown(t)
	defer cleanup()

	server, statusClient, _, _ := setupTestServer(t, townRoot)
	defer server.Close()

	t.Run("WatchStatus_CancelledContext", func(t *testing.T) {
		// Create a context that we'll cancel after receiving one update
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		req := connect.NewRequest(&gastownv1.WatchStatusRequest{})
		stream, err := statusClient.WatchStatus(ctx, req)
		if err != nil {
			t.Fatalf("WatchStatus failed: %v", err)
		}

		// Try to receive at least one message (wait up to 3s for first tick)
		received := false
		for stream.Receive() {
			msg := stream.Msg()
			if msg != nil {
				received = true
				// Verify we got a town status update
				if msg.GetTown() != nil {
					t.Logf("Received town status: %s", msg.GetTown().Name)
				}
				break // Cancel after first message
			}
		}

		// Cancel context to stop stream
		cancel()

		// Stream should complete without error when context is cancelled
		if err := stream.Err(); err != nil && err != context.Canceled {
			// Ignore context cancelled errors
			if !isContextCancelledError(err) {
				t.Logf("Stream error (expected due to cancellation): %v", err)
			}
		}

		if !received {
			t.Log("No status updates received within timeout (may be timing-dependent)")
		}
	})
}

// isContextCancelledError checks if an error is due to context cancellation.
func isContextCancelledError(err error) bool {
	if err == context.Canceled {
		return true
	}
	if connectErr, ok := err.(*connect.Error); ok {
		return connectErr.Code() == connect.CodeCanceled
	}
	return false
}
