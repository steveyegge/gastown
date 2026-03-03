package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discardLogger returns a slog.Logger that discards all output (keeps test output clean).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew(t *testing.T) {
	t.Run("empty TownRoot returns error", func(t *testing.T) {
		_, err := New(Config{Logger: discardLogger()}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TownRoot")
	})

	t.Run("relative TownRoot returns error", func(t *testing.T) {
		_, err := New(Config{TownRoot: "relative/path", Logger: discardLogger()}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TownRoot")
	})

	t.Run("empty AllowedCommands logs warning and produces empty map", func(t *testing.T) {
		// New() logs a warning but must not panic or error (other than TownRoot).
		srv, err := New(Config{TownRoot: t.TempDir(), Logger: discardLogger()}, nil)
		require.NoError(t, err)
		assert.NotNil(t, srv)
		assert.Empty(t, srv.allowed)
	})

	t.Run("AllowedCommands are stored in allowed map", func(t *testing.T) {
		srv, err := New(Config{TownRoot: t.TempDir(), AllowedCommands: []string{"echo", "true"}, Logger: discardLogger()}, nil)
		require.NoError(t, err)
		assert.True(t, srv.allowed["echo"])
		assert.True(t, srv.allowed["true"])
		assert.False(t, srv.allowed["curl"])
	})

	t.Run("isAllowed reflects allowed map", func(t *testing.T) {
		srv, err := New(Config{TownRoot: t.TempDir(), AllowedCommands: []string{"echo", "true"}, Logger: discardLogger()}, nil)
		require.NoError(t, err)
		assert.True(t, srv.isAllowed("echo"))
		assert.True(t, srv.isAllowed("true"))
		assert.False(t, srv.isAllowed("curl"))
		assert.False(t, srv.isAllowed(""))
	})

	t.Run("AllowedCommands with path separators are rejected", func(t *testing.T) {
		srv, err := New(Config{TownRoot: t.TempDir(), AllowedCommands: []string{"/usr/bin/echo", "true", `C:\echo.exe`}, Logger: discardLogger()}, nil)
		require.NoError(t, err)
		assert.False(t, srv.isAllowed("/usr/bin/echo"), "absolute path should be rejected")
		assert.True(t, srv.isAllowed("true"), "plain name should be accepted")
		assert.False(t, srv.isAllowed(`C:\echo.exe`), "windows path should be rejected")
	})
}

func TestMinimalEnv(t *testing.T) {
	t.Run("contains HOME and PATH when set", func(t *testing.T) {
		// Ensure HOME and PATH are set (they almost always are, but be safe).
		if os.Getenv("HOME") == "" {
			t.Setenv("HOME", "/tmp/testhome")
		}
		if os.Getenv("PATH") == "" {
			t.Setenv("PATH", "/usr/bin:/bin")
		}

		env := minimalEnv()
		hasHome, hasPath := false, false
		for _, e := range env {
			if strings.HasPrefix(e, "HOME=") {
				hasHome = true
			}
			if strings.HasPrefix(e, "PATH=") {
				hasPath = true
			}
		}
		assert.True(t, hasHome, "minimalEnv should include HOME")
		assert.True(t, hasPath, "minimalEnv should include PATH")
	})

	t.Run("does NOT contain arbitrary test env vars", func(t *testing.T) {
		t.Setenv("PROXY_TEST_SENTINEL_ENV", "secret_value_xyz")

		env := minimalEnv()
		for _, e := range env {
			assert.False(t, strings.Contains(e, "PROXY_TEST_SENTINEL_ENV"),
				"minimalEnv should not contain test sentinel env var, got: %s", e)
		}
	})

	t.Run("omits keys that are empty", func(t *testing.T) {
		// Temporarily unset HOME to verify it is omitted when empty.
		orig := os.Getenv("HOME")
		require.NoError(t, os.Unsetenv("HOME"))
		t.Cleanup(func() {
			if orig != "" {
				os.Setenv("HOME", orig) //nolint:errcheck
			}
		})

		env := minimalEnv()
		for _, e := range env {
			assert.False(t, strings.HasPrefix(e, "HOME="),
				"empty HOME should be omitted, got: %s", e)
		}
	})
}

func TestExtraSANIPs(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	extraIP := net.ParseIP("192.0.2.99")
	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		ExtraSANIPs:     []net.IP{extraIP},
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { srv.Start(ctx) }() //nolint:errcheck

	var addr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			addr = a.String()
			return true
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, addr, 5*time.Second)

	// Dial and inspect the server cert for the extra IP SAN.
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		RootCAs:    pool,
		ServerName: "gt-proxy-server",
	})
	require.NoError(t, err)
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	require.NotEmpty(t, certs)
	serverCert := certs[0]

	found := false
	for _, ip := range serverCert.IPAddresses {
		if ip.Equal(extraIP) {
			found = true
			break
		}
	}
	assert.True(t, found, "extra IP SAN 192.0.2.99 should appear in server cert IPAddresses; got: %v", serverCert.IPAddresses)
}

func TestExtraSANHosts(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	extraHost := "proxy.example.com"
	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		ExtraSANHosts:   []string{extraHost},
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { srv.Start(ctx) }() //nolint:errcheck

	var addr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			addr = a.String()
			return true
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, addr, 5*time.Second)

	// Dial and inspect the server cert for the extra DNS SAN.
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		RootCAs:    pool,
		ServerName: "gt-proxy-server",
	})
	require.NoError(t, err)
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	require.NotEmpty(t, certs)
	serverCert := certs[0]

	found := false
	for _, name := range serverCert.DNSNames {
		if name == extraHost {
			found = true
			break
		}
	}
	assert.True(t, found, "extra DNS SAN %q should appear in server cert DNSNames; got: %v", extraHost, serverCert.DNSNames)
}

// TestServerListenIPs exercises each branch of serverListenIPs directly.
func TestServerListenIPs(t *testing.T) {
	// containsIP reports whether any IP in ips equals the IP parsed from s.
	containsIP := func(ips []net.IP, s string) bool {
		target := net.ParseIP(s)
		for _, ip := range ips {
			if ip.Equal(target) {
				return true
			}
		}
		return false
	}

	t.Run("parse error returns loopback IPs", func(t *testing.T) {
		// "not-an-addr" has no port — SplitHostPort fails.
		ips := serverListenIPs("not-an-addr")
		assert.Len(t, ips, 2, "parse error should return exactly [127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})

	t.Run("loopback IPv4 listen addr returns loopback IPs", func(t *testing.T) {
		ips := serverListenIPs("127.0.0.1:1234")
		assert.Len(t, ips, 2, "loopback addr should return exactly [127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})

	t.Run("loopback IPv6 listen addr returns loopback IPs", func(t *testing.T) {
		ips := serverListenIPs("[::1]:1234")
		assert.Len(t, ips, 2, "IPv6 loopback addr should return exactly [127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})

	t.Run("specific non-loopback IP includes that IP and both loopbacks", func(t *testing.T) {
		// 192.0.2.1 is TEST-NET-1 (RFC 5737) — documentation-only, never routed.
		ips := serverListenIPs("192.0.2.1:1234")
		assert.Len(t, ips, 3, "specific IP should return exactly [ip, 127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "192.0.2.1"), "should contain the specific IP")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})

	t.Run("0.0.0.0 enumerates interfaces and always includes loopbacks", func(t *testing.T) {
		ips := serverListenIPs("0.0.0.0:1234")
		assert.GreaterOrEqual(t, len(ips), 2, "0.0.0.0 should return at least [127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})

	t.Run("IPv6 unspecified enumerates interfaces and always includes loopbacks", func(t *testing.T) {
		ips := serverListenIPs("[::]:1234")
		assert.GreaterOrEqual(t, len(ips), 2, "[::] should return at least [127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})

	t.Run("hostname returns loopback IPs", func(t *testing.T) {
		// net.ParseIP("myhostname") is nil → hostname branch returns loopback only.
		ips := serverListenIPs("myhostname:1234")
		assert.Len(t, ips, 2, "hostname should return exactly [127.0.0.1, ::1]")
		assert.True(t, containsIP(ips, "127.0.0.1"))
		assert.True(t, containsIP(ips, "::1"))
	})
}

// waitForServer polls addr until a TCP connection succeeds or timeout elapses.
func waitForServer(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %v", addr, timeout)
}

func TestStartIntegration(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	startErr := make(chan error, 1)
	go func() {
		startErr <- srv.Start(ctx)
	}()

	// Wait until the listener is bound and the address is available via Addr().
	var addr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			addr = a.String()
			return true
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "server Addr() should become non-nil after Start()")
	waitForServer(t, addr, 5*time.Second)

	// Build a CA pool and an authorised client cert.
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)

	clientCertPEM, clientKeyPEM, err := ca.IssuePolecat("gt-gastown-testclient", time.Hour)
	require.NoError(t, err)
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	require.NoError(t, err)

	authorisedClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      pool,
			},
		},
	}

	t.Run("authorised client can POST exec and get output", func(t *testing.T) {
		body := strings.NewReader(`{"argv":["echo","hello"]}`)
		resp, err := authorisedClient.Post(
			"https://"+addr+"/v1/exec",
			"application/json",
			body,
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var result execResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, 0, result.ExitCode)
		assert.Contains(t, result.Stdout, "hello")
	})

	t.Run("client with no certificate is rejected at TLS handshake", func(t *testing.T) {
		noCertClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: pool,
					// No Certificates — no client cert presented.
				},
			},
		}
		_, err := noCertClient.Post(
			"https://"+addr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hi"]}`),
		)
		assert.Error(t, err, "connection without client cert should be rejected")
	})

	t.Run("client cert signed by different CA is rejected", func(t *testing.T) {
		// Generate a completely independent CA.
		dir2 := t.TempDir()
		ca2, err := GenerateCA(dir2)
		require.NoError(t, err)

		wrongCertPEM, wrongKeyPEM, err := ca2.IssuePolecat("gt-gastown-evil", time.Hour)
		require.NoError(t, err)
		wrongClientCert, err := tls.X509KeyPair(wrongCertPEM, wrongKeyPEM)
		require.NoError(t, err)

		wrongClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{wrongClientCert},
					RootCAs:      pool,
				},
			},
		}
		_, err = wrongClient.Post(
			"https://"+addr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hi"]}`),
		)
		assert.Error(t, err, "client cert from different CA should be rejected")
	})

	t.Run("cancelling context causes server to shut down", func(t *testing.T) {
		cancel()
		// Allow the server goroutine to finish shutting down.
		select {
		case err := <-startErr:
			// Shutdown returns nil (graceful) or ErrServerClosed — both are fine.
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("server did not shut down within 5s after context cancellation")
		}

		// Subsequent requests must fail (server is gone); use a fresh client to avoid
		// reusing cached connections.
		freshClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      pool,
				},
			},
		}
		_, err := freshClient.Post(
			"https://"+addr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hi"]}`),
		)
		assert.Error(t, err, "requests after shutdown should fail")
	})
}

// TestCertRevocation verifies that a cert added to the deny list is rejected at
// the TLS handshake and that other (non-revoked) certs continue to work.
func TestCertRevocation(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { srv.Start(ctx) }() //nolint:errcheck

	var addr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			addr = a.String()
			return true
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, addr, 5*time.Second)

	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)

	// Issue two distinct polecat certs.
	cert1PEM, key1PEM, err := ca.IssuePolecat("gt-gastown-alice", time.Hour)
	require.NoError(t, err)
	tlsCert1, err := tls.X509KeyPair(cert1PEM, key1PEM)
	require.NoError(t, err)

	cert2PEM, key2PEM, err := ca.IssuePolecat("gt-gastown-bob", time.Hour)
	require.NoError(t, err)
	tlsCert2, err := tls.X509KeyPair(cert2PEM, key2PEM)
	require.NoError(t, err)

	makeClient := func(cert tls.Certificate) *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
					RootCAs:      pool,
				},
			},
		}
	}

	// Both certs work before any revocation.
	for _, c := range []tls.Certificate{tlsCert1, tlsCert2} {
		_, err := makeClient(c).Post(
			"https://"+addr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hi"]}`),
		)
		require.NoError(t, err, "both certs should work before revocation")
	}

	// Parse cert1's serial number and revoke it via DenyCert.
	leaf1, err := x509.ParseCertificate(tlsCert1.Certificate[0])
	require.NoError(t, err)
	srv.DenyCert(leaf1.SerialNumber)

	// cert1 is now denied — TLS handshake must fail.
	_, err = makeClient(tlsCert1).Post(
		"https://"+addr+"/v1/exec",
		"application/json",
		strings.NewReader(`{"argv":["echo","hi"]}`),
	)
	assert.Error(t, err, "revoked cert1 should be rejected at TLS handshake")

	// cert2 is not revoked — it must still work.
	resp, err := makeClient(tlsCert2).Post(
		"https://"+addr+"/v1/exec",
		"application/json",
		strings.NewReader(`{"argv":["echo","hi"]}`),
	)
	require.NoError(t, err, "non-revoked cert2 should still work")
	resp.Body.Close()
}

// TestAdminDenyCertEndpoint verifies the local admin HTTP endpoint for revoking certs.
func TestAdminDenyCertEndpoint(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AdminListenAddr: "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { srv.Start(ctx) }() //nolint:errcheck

	// Wait for both listeners to be bound.
	var mainAddr, adminAddr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			mainAddr = a.String()
		}
		if a := srv.AdminAddr(); a != nil {
			adminAddr = a.String()
		}
		return mainAddr != "" && adminAddr != ""
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, mainAddr, 5*time.Second)
	waitForServer(t, adminAddr, 5*time.Second)

	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)

	// Issue a cert for testing.
	certPEM, keyPEM, err := ca.IssuePolecat("gt-gastown-carol", time.Hour)
	require.NoError(t, err)
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	require.NoError(t, err)
	serialHex := leaf.SerialNumber.Text(16)

	mTLSClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				RootCAs:      pool,
			},
		},
	}
	adminClient := &http.Client{}

	t.Run("cert works before revocation", func(t *testing.T) {
		_, err := mTLSClient.Post(
			"https://"+mainAddr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hi"]}`),
		)
		require.NoError(t, err)
	})

	t.Run("GET to admin endpoint returns 405", func(t *testing.T) {
		resp, err := adminClient.Get("http://" + adminAddr + "/v1/admin/deny-cert")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/deny-cert",
			"application/json",
			strings.NewReader("{not json}"),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("missing serial returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/deny-cert",
			"application/json",
			strings.NewReader(`{}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("invalid serial returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/deny-cert",
			"application/json",
			strings.NewReader(`{"serial":"not-hex-!@#"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("valid serial returns 204 and cert is rejected", func(t *testing.T) {
		// Revoke carol's cert via the admin API.
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/deny-cert",
			"application/json",
			strings.NewReader(`{"serial":"`+serialHex+`"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		// carol's cert must now be rejected at the TLS handshake.
		// Use a fresh client to avoid reusing a cached connection.
		freshMTLS := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{tlsCert},
					RootCAs:      pool,
				},
			},
		}
		_, err = freshMTLS.Post(
			"https://"+mainAddr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hi"]}`),
		)
		assert.Error(t, err, "revoked cert should be rejected at TLS handshake")
	})
}

// TestAdminIssueCertEndpoint verifies the local admin HTTP endpoint for issuing polecat certs.
func TestAdminIssueCertEndpoint(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA(dir)
	require.NoError(t, err)

	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AdminListenAddr: "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { srv.Start(ctx) }() //nolint:errcheck

	var mainAddr, adminAddr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			mainAddr = a.String()
		}
		if a := srv.AdminAddr(); a != nil {
			adminAddr = a.String()
		}
		return mainAddr != "" && adminAddr != ""
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, mainAddr, 5*time.Second)
	waitForServer(t, adminAddr, 5*time.Second)

	adminClient := &http.Client{}

	t.Run("GET returns 405", func(t *testing.T) {
		resp, err := adminClient.Get("http://" + adminAddr + "/v1/admin/issue-cert")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("missing rig returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/issue-cert",
			"application/json",
			strings.NewReader(`{"name":"rust"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/issue-cert",
			"application/json",
			strings.NewReader(`{"rig":"MyRig"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("invalid ttl returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/issue-cert",
			"application/json",
			strings.NewReader(`{"rig":"MyRig","name":"rust","ttl":"bogus"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("negative ttl returns 400", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/issue-cert",
			"application/json",
			strings.NewReader(`{"rig":"MyRig","name":"rust","ttl":"-1h"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("valid request issues cert that works with mTLS server", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/issue-cert",
			"application/json",
			strings.NewReader(`{"rig":"MyRig","name":"rust","ttl":"1h"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result issueCertResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, "gt-MyRig-rust", result.CN)
		assert.NotEmpty(t, result.Cert)
		assert.NotEmpty(t, result.Key)
		assert.NotEmpty(t, result.CA)
		assert.NotEmpty(t, result.Serial)
		assert.NotEmpty(t, result.ExpiresAt)

		// Use the issued cert to make an mTLS request to the main server.
		clientCert, err := tls.X509KeyPair([]byte(result.Cert), []byte(result.Key))
		require.NoError(t, err)

		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM([]byte(result.CA))
		require.True(t, ok, "returned CA PEM should be valid")

		mTLSClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      pool,
				},
			},
		}
		execResp, err := mTLSClient.Post(
			"https://"+mainAddr+"/v1/exec",
			"application/json",
			strings.NewReader(`{"argv":["echo","hello"]}`),
		)
		require.NoError(t, err)
		defer execResp.Body.Close()
		assert.Equal(t, http.StatusOK, execResp.StatusCode)

		var execResult execResponse
		require.NoError(t, json.NewDecoder(execResp.Body).Decode(&execResult))
		assert.Equal(t, 0, execResult.ExitCode)
		assert.Contains(t, execResult.Stdout, "hello")
	})

	t.Run("default ttl is used when not specified", func(t *testing.T) {
		resp, err := adminClient.Post(
			"http://"+adminAddr+"/v1/admin/issue-cert",
			"application/json",
			strings.NewReader(`{"rig":"MyRig","name":"furiosa"}`),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result issueCertResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, "gt-MyRig-furiosa", result.CN)

		expiry, err := time.Parse(time.RFC3339, result.ExpiresAt)
		require.NoError(t, err)
		// Default TTL is 720h (30 days). Allow some clock skew.
		expectedExpiry := time.Now().Add(720 * time.Hour)
		assert.WithinDuration(t, expectedExpiry, expiry, 5*time.Minute)
	})
}
