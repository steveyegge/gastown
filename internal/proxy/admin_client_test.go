package proxy

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminClient_NilSafe(t *testing.T) {
	var c *AdminClient

	t.Run("IssueCert returns nil on nil client", func(t *testing.T) {
		result, err := c.IssueCert(context.Background(), "rig", "name", "720h")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("DenyCert returns nil on nil client", func(t *testing.T) {
		err := c.DenyCert(context.Background(), "abc123")
		assert.NoError(t, err)
	})

	t.Run("Ping returns nil on nil client", func(t *testing.T) {
		err := c.Ping(context.Background())
		assert.NoError(t, err)
	})
}

func TestAdminClient_IssueCert(t *testing.T) {
	t.Run("successful issue", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/v1/admin/issue-cert", r.URL.Path)

			var req issueCertRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "myrig", req.Rig)
			assert.Equal(t, "ruby", req.Name)
			assert.Equal(t, "720h", req.TTL)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(IssueCertResult{
				CN:        "gt-myrig-ruby",
				Cert:      "---CERT---",
				Key:       "---KEY---",
				CA:        "---CA---",
				Serial:    "deadbeef",
				ExpiresAt: "2026-04-03T00:00:00Z",
			})
		}))
		defer srv.Close()

		// Strip "http://" prefix since NewAdminClient adds it
		addr := srv.Listener.Addr().String()
		c := NewAdminClient(addr)

		result, err := c.IssueCert(context.Background(), "myrig", "ruby", "720h")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "gt-myrig-ruby", result.CN)
		assert.Equal(t, "deadbeef", result.Serial)
		assert.Equal(t, "---CERT---", result.Cert)
		assert.Equal(t, "---KEY---", result.Key)
		assert.Equal(t, "---CA---", result.CA)
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request: rig is required", http.StatusBadRequest)
		}))
		defer srv.Close()

		c := NewAdminClient(srv.Listener.Addr().String())
		result, err := c.IssueCert(context.Background(), "", "ruby", "720h")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status 400")
	})

	t.Run("unreachable server", func(t *testing.T) {
		c := NewAdminClient("127.0.0.1:1") // port 1 should be unreachable
		result, err := c.IssueCert(context.Background(), "rig", "name", "720h")
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestAdminClient_DenyCert(t *testing.T) {
	t.Run("successful deny", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/v1/admin/deny-cert", r.URL.Path)

			var req denyCertRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Equal(t, "deadbeef", req.Serial)

			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		c := NewAdminClient(srv.Listener.Addr().String())
		err := c.DenyCert(context.Background(), "deadbeef")
		assert.NoError(t, err)
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad request: serial is required", http.StatusBadRequest)
		}))
		defer srv.Close()

		c := NewAdminClient(srv.Listener.Addr().String())
		err := c.DenyCert(context.Background(), "")
		assert.Error(t, err)
	})

	t.Run("unreachable server", func(t *testing.T) {
		c := NewAdminClient("127.0.0.1:1")
		err := c.DenyCert(context.Background(), "deadbeef")
		assert.Error(t, err)
	})
}

// TestCertLifecycle_IssueExtractDeny verifies the full cert lifecycle:
// 1. Issue a polecat cert via CA
// 2. Extract the serial number (same as addDaytona does)
// 3. Use that serial to deny the cert via AdminClient
// This is an integration test of the cert issuance → storage → revocation path.
func TestCertLifecycle_IssueExtractDeny(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	// Step 1: Issue polecat cert (same TTL as addDaytona: 720h / 30 days).
	certCN := "gt-myrig-onyx"
	certPEM, keyPEM, err := ca.IssuePolecat(certCN, 720*time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, certPEM)
	require.NotEmpty(t, keyPEM)

	// Step 2: Extract serial (same code path as addDaytona).
	var certSerial string
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block, "certPEM should decode")
	leaf, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	certSerial = leaf.SerialNumber.Text(16)
	require.NotEmpty(t, certSerial, "extracted serial should be non-empty")

	// Step 3: Deny cert via AdminClient (mock server).
	var deniedSerial string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/admin/deny-cert" {
			var req denyCertRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			deniedSerial = req.Serial
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewAdminClient(srv.Listener.Addr().String())
	err = client.DenyCert(context.Background(), certSerial)
	require.NoError(t, err)

	// Verify the serial that was denied matches what was extracted.
	assert.Equal(t, certSerial, deniedSerial,
		"deny-cert should receive the same serial extracted from the issued cert")

	// Verify the cert verifies against the CA (sanity check).
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	assert.NoError(t, err, "issued cert should verify against CA")
}

func TestAdminClient_Ping(t *testing.T) {
	t.Run("server reachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/admin/health", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := NewAdminClient(srv.Listener.Addr().String())
		err := c.Ping(context.Background())
		assert.NoError(t, err)
	})

	t.Run("non-2xx treated as unhealthy", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			status int
		}{
			{"500 Internal Server Error", http.StatusInternalServerError},
			{"503 Service Unavailable", http.StatusServiceUnavailable},
			{"404 Not Found", http.StatusNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "unhealthy", tc.status)
				}))
				defer srv.Close()

				c := NewAdminClient(srv.Listener.Addr().String())
				err := c.Ping(context.Background())
				assert.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("status %d", tc.status))
			})
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		c := NewAdminClient("127.0.0.1:1")
		err := c.Ping(context.Background())
		assert.Error(t, err)
	})
}
