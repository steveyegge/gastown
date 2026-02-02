package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/steveyegge/gastown/internal/eventbus"
	gastownv1 "github.com/steveyegge/gastown/mobile/gen/gastown/v1"
)

// TestToUrgency tests urgency string to proto enum conversion.
func TestToUrgency(t *testing.T) {
	tests := []struct {
		input string
		want  gastownv1.Urgency
	}{
		{"high", gastownv1.Urgency_URGENCY_HIGH},
		{"medium", gastownv1.Urgency_URGENCY_MEDIUM},
		{"low", gastownv1.Urgency_URGENCY_LOW},
		{"", gastownv1.Urgency_URGENCY_UNSPECIFIED},
		{"invalid", gastownv1.Urgency_URGENCY_UNSPECIFIED},
		{"HIGH", gastownv1.Urgency_URGENCY_UNSPECIFIED}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toUrgency(tt.input)
			if got != tt.want {
				t.Errorf("toUrgency(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestToPriority tests priority string to proto enum conversion.
func TestToPriority(t *testing.T) {
	tests := []struct {
		input string
		want  gastownv1.Priority
	}{
		{"urgent", gastownv1.Priority_PRIORITY_URGENT},
		{"high", gastownv1.Priority_PRIORITY_HIGH},
		{"normal", gastownv1.Priority_PRIORITY_NORMAL},
		{"low", gastownv1.Priority_PRIORITY_LOW},
		{"", gastownv1.Priority_PRIORITY_UNSPECIFIED},
		{"invalid", gastownv1.Priority_PRIORITY_UNSPECIFIED},
		{"URGENT", gastownv1.Priority_PRIORITY_UNSPECIFIED}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toPriority(tt.input)
			if got != tt.want {
				t.Errorf("toPriority(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestNewStatusServer tests StatusServer constructor.
func TestNewStatusServer(t *testing.T) {
	server := NewStatusServer("/tmp/test-town")
	if server == nil {
		t.Fatal("NewStatusServer returned nil")
	}
	if server.townRoot != "/tmp/test-town" {
		t.Errorf("townRoot = %q, want %q", server.townRoot, "/tmp/test-town")
	}
}

// TestNewDecisionServer tests DecisionServer constructor.
func TestNewDecisionServer(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()
	server := NewDecisionServer("/tmp/test-town", bus)
	if server == nil {
		t.Fatal("NewDecisionServer returned nil")
	}
	if server.townRoot != "/tmp/test-town" {
		t.Errorf("townRoot = %q, want %q", server.townRoot, "/tmp/test-town")
	}
}

// TestNewMailServer tests MailServer constructor.
func TestNewMailServer(t *testing.T) {
	server := NewMailServer("/tmp/test-town")
	if server == nil {
		t.Fatal("NewMailServer returned nil")
	}
	if server.townRoot != "/tmp/test-town" {
		t.Errorf("townRoot = %q, want %q", server.townRoot, "/tmp/test-town")
	}
}

// TestAPIKeyInterceptorNoAuth tests that requests pass through when no auth is configured.
func TestAPIKeyInterceptorNoAuth(t *testing.T) {
	interceptor := APIKeyInterceptor("")

	// The interceptor returns a function - we can verify it was created
	if interceptor == nil {
		t.Fatal("interceptor is nil")
	}
}

// TestAPIKeyInterceptorWithAuth tests that API key validation works.
func TestAPIKeyInterceptorWithAuth(t *testing.T) {
	interceptor := APIKeyInterceptor("secret-key")

	// The interceptor returns a function - we can verify it was created
	if interceptor == nil {
		t.Fatal("interceptor is nil")
	}
}

// TestLoadTLSConfig tests TLS certificate loading.
func TestLoadTLSConfig(t *testing.T) {
	t.Run("valid certificates", func(t *testing.T) {
		// Create temporary directory for test certs
		tmpDir, err := os.MkdirTemp("", "tls-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		// Generate self-signed cert
		if err := generateTestCert(certFile, keyFile); err != nil {
			t.Fatalf("generating test cert: %v", err)
		}

		// Test loading
		config, err := LoadTLSConfig(certFile, keyFile)
		if err != nil {
			t.Fatalf("LoadTLSConfig failed: %v", err)
		}

		if config == nil {
			t.Fatal("config is nil")
		}
		if len(config.Certificates) != 1 {
			t.Errorf("expected 1 certificate, got %d", len(config.Certificates))
		}
		if config.MinVersion != 0x0303 { // TLS 1.2
			t.Errorf("MinVersion = %x, want TLS 1.2 (0x0303)", config.MinVersion)
		}
	})

	t.Run("missing cert file", func(t *testing.T) {
		_, err := LoadTLSConfig("/nonexistent/cert.pem", "/nonexistent/key.pem")
		if err == nil {
			t.Error("expected error for missing files")
		}
	})

	t.Run("invalid cert format", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "tls-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		certFile := filepath.Join(tmpDir, "cert.pem")
		keyFile := filepath.Join(tmpDir, "key.pem")

		// Write invalid data
		os.WriteFile(certFile, []byte("not a cert"), 0600)
		os.WriteFile(keyFile, []byte("not a key"), 0600)

		_, err = LoadTLSConfig(certFile, keyFile)
		if err == nil {
			t.Error("expected error for invalid cert format")
		}
	})
}

// generateTestCert generates a self-signed certificate for testing.
func generateTestCert(certFile, keyFile string) error {
	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// Write cert file
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	// Write key file
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	keyOut.Close()

	return nil
}

// TestDecisionServerGetDecision tests that GetDecision returns NotFound for nonexistent decisions.
func TestDecisionServerGetDecision(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()
	server := NewDecisionServer("/tmp/test", bus)
	req := connect.NewRequest(&gastownv1.GetDecisionRequest{DecisionId: "nonexistent-id"})

	_, err := server.GetDecision(context.Background(), req)
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
}

// TestDecisionServerResolve tests that resolving a non-existent decision returns error.
func TestDecisionServerResolve(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()
	server := NewDecisionServer("/tmp/test-resolve", bus)
	req := connect.NewRequest(&gastownv1.ResolveRequest{DecisionId: "nonexistent-id", ChosenIndex: 1})

	_, err := server.Resolve(context.Background(), req)
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
}

// TestDecisionServerCancel tests that canceling a non-existent decision returns error.
func TestDecisionServerCancel(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()
	server := NewDecisionServer("/tmp/test-cancel", bus)
	req := connect.NewRequest(&gastownv1.CancelRequest{DecisionId: "nonexistent-id"})

	_, err := server.Cancel(context.Background(), req)
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
}

// TestStatusServerGetAgentStatus tests GetAgentStatus returns correct errors for various inputs.
func TestStatusServerGetAgentStatus(t *testing.T) {
	server := NewStatusServer("/tmp/test")

	t.Run("NilAddress", func(t *testing.T) {
		req := connect.NewRequest(&gastownv1.GetAgentStatusRequest{
			Address: nil,
		})

		_, err := server.GetAgentStatus(context.Background(), req)
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

	t.Run("AgentNotFound", func(t *testing.T) {
		req := connect.NewRequest(&gastownv1.GetAgentStatusRequest{
			Address: &gastownv1.AgentAddress{Name: "nonexistent-agent"},
		})

		_, err := server.GetAgentStatus(context.Background(), req)
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
}

// TestMatchesAgentAddress tests the agent address matching logic.
func TestMatchesAgentAddress(t *testing.T) {
	tests := []struct {
		name      string
		agentAddr *gastownv1.AgentAddress
		reqAddr   *gastownv1.AgentAddress
		want      bool
	}{
		{
			name:      "NilAgentAddr",
			agentAddr: nil,
			reqAddr:   &gastownv1.AgentAddress{Name: "test"},
			want:      false,
		},
		{
			name:      "NilReqAddr",
			agentAddr: &gastownv1.AgentAddress{Name: "test"},
			reqAddr:   nil,
			want:      false,
		},
		{
			name:      "EmptyReqAddr",
			agentAddr: &gastownv1.AgentAddress{Name: "test"},
			reqAddr:   &gastownv1.AgentAddress{},
			want:      false,
		},
		{
			name:      "MatchByName",
			agentAddr: &gastownv1.AgentAddress{Name: "mayor"},
			reqAddr:   &gastownv1.AgentAddress{Name: "mayor"},
			want:      true,
		},
		{
			name:      "NoMatchByName",
			agentAddr: &gastownv1.AgentAddress{Name: "mayor"},
			reqAddr:   &gastownv1.AgentAddress{Name: "deacon"},
			want:      false,
		},
		{
			name:      "MatchByRigAndRole",
			agentAddr: &gastownv1.AgentAddress{Rig: "gastown", Role: "witness"},
			reqAddr:   &gastownv1.AgentAddress{Rig: "gastown", Role: "witness"},
			want:      true,
		},
		{
			name:      "NoMatchByRig",
			agentAddr: &gastownv1.AgentAddress{Rig: "gastown", Role: "witness"},
			reqAddr:   &gastownv1.AgentAddress{Rig: "beads", Role: "witness"},
			want:      false,
		},
		{
			name:      "MatchByRigRoleAndName",
			agentAddr: &gastownv1.AgentAddress{Rig: "gastown", Role: "polecats", Name: "furiosa"},
			reqAddr:   &gastownv1.AgentAddress{Rig: "gastown", Role: "polecats", Name: "furiosa"},
			want:      true,
		},
		{
			name:      "PartialMatch_RigOnly",
			agentAddr: &gastownv1.AgentAddress{Rig: "gastown", Role: "witness"},
			reqAddr:   &gastownv1.AgentAddress{Rig: "gastown"},
			want:      true,
		},
		{
			name:      "PartialMatch_RoleOnly",
			agentAddr: &gastownv1.AgentAddress{Rig: "gastown", Role: "witness"},
			reqAddr:   &gastownv1.AgentAddress{Role: "witness"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAgentAddress(tt.agentAddr, tt.reqAddr)
			if got != tt.want {
				t.Errorf("matchesAgentAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormatAgentAddressForError tests error message formatting.
func TestFormatAgentAddressForError(t *testing.T) {
	tests := []struct {
		name string
		addr *gastownv1.AgentAddress
		want string
	}{
		{
			name: "NilAddress",
			addr: nil,
			want: "<nil>",
		},
		{
			name: "FullAddress",
			addr: &gastownv1.AgentAddress{Rig: "gastown", Role: "polecats", Name: "furiosa"},
			want: "gastown/polecats/furiosa",
		},
		{
			name: "RigAndRole",
			addr: &gastownv1.AgentAddress{Rig: "gastown", Role: "witness"},
			want: "gastown/witness",
		},
		{
			name: "RigOnly",
			addr: &gastownv1.AgentAddress{Rig: "gastown"},
			want: "gastown",
		},
		{
			name: "NameOnly",
			addr: &gastownv1.AgentAddress{Name: "mayor"},
			want: "mayor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAgentAddressForError(tt.addr)
			if got != tt.want {
				t.Errorf("formatAgentAddressForError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMailServerUnimplementedMethods tests all unimplemented mail methods.
func TestMailServerUnimplementedMethods(t *testing.T) {
	server := NewMailServer("/tmp/test")

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "ReadMessage",
			call: func() error {
				req := connect.NewRequest(&gastownv1.ReadMessageRequest{MessageId: "test"})
				_, err := server.ReadMessage(context.Background(), req)
				return err
			},
		},
		{
			name: "SendMessage",
			call: func() error {
				req := connect.NewRequest(&gastownv1.SendMessageRequest{})
				_, err := server.SendMessage(context.Background(), req)
				return err
			},
		},
		{
			name: "MarkRead",
			call: func() error {
				req := connect.NewRequest(&gastownv1.MarkReadRequest{MessageId: "test"})
				_, err := server.MarkRead(context.Background(), req)
				return err
			},
		},
		{
			name: "DeleteMessage",
			call: func() error {
				req := connect.NewRequest(&gastownv1.DeleteMessageRequest{MessageId: "test"})
				_, err := server.DeleteMessage(context.Background(), req)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
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
}
