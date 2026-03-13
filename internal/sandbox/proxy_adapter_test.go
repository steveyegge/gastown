package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/steveyegge/gastown/internal/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyAdminAdapter_CompileTimeAssertion(t *testing.T) {
	// The var _ CertIssuer = (*ProxyAdminAdapter)(nil) line ensures this
	// at compile time, but this test documents the intent explicitly.
	var ci CertIssuer = NewProxyAdminAdapter(nil)
	_ = ci
}

func TestProxyAdminAdapter_NilAdmin(t *testing.T) {
	adapter := NewProxyAdminAdapter(nil)

	t.Run("IssueCert returns error", func(t *testing.T) {
		result, err := adapter.IssueCert(context.Background(), "rig", "name", "720h")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "proxy admin client is nil")
	})

	t.Run("DenyCert returns error", func(t *testing.T) {
		err := adapter.DenyCert(context.Background(), "abc123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "proxy admin client is nil")
	})
}

func TestProxyAdminAdapter_IssueCert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/admin/issue-cert" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"cn":         "gt-testrig-polecat1",
			"cert":       "CERT_PEM",
			"key":        "KEY_PEM",
			"ca":         "CA_PEM",
			"serial":     "deadbeef",
			"expires_at": "2026-04-12T00:00:00Z",
		})
	}))
	defer srv.Close()

	// Extract host:port from test server URL (strip "http://").
	addr := srv.URL[len("http://"):]
	client := proxy.NewAdminClient(addr)
	adapter := NewProxyAdminAdapter(client)

	result, err := adapter.IssueCert(context.Background(), "testrig", "polecat1", "720h")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "gt-testrig-polecat1", result.CN)
	assert.Equal(t, "CERT_PEM", result.Cert)
	assert.Equal(t, "KEY_PEM", result.Key)
	assert.Equal(t, "CA_PEM", result.CA)
	assert.Equal(t, "deadbeef", result.Serial)
	assert.Equal(t, "2026-04-12T00:00:00Z", result.ExpiresAt)
}

func TestProxyAdminAdapter_DenyCert(t *testing.T) {
	var receivedSerial string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/admin/deny-cert" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Serial string `json:"serial"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedSerial = req.Serial
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	addr := srv.URL[len("http://"):]
	client := proxy.NewAdminClient(addr)
	adapter := NewProxyAdminAdapter(client)

	err := adapter.DenyCert(context.Background(), "abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", receivedSerial)
}

func TestProxyAdminAdapter_IssueCertServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	addr := srv.URL[len("http://"):]
	client := proxy.NewAdminClient(addr)
	adapter := NewProxyAdminAdapter(client)

	result, err := adapter.IssueCert(context.Background(), "rig", "name", "720h")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "500")
}

func TestProxyAdminAdapter_DenyCertServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	addr := srv.URL[len("http://"):]
	client := proxy.NewAdminClient(addr)
	adapter := NewProxyAdminAdapter(client)

	err := adapter.DenyCert(context.Background(), "bad-serial")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
